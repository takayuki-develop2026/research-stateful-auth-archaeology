<?php

namespace App\Modules\Payment\Application\UseCase;

use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Domain\Event\DomainPaymentEventType;
use App\Modules\Payment\Domain\Repository\PaymentQueryRepository;
use App\Modules\Payment\Domain\Repository\PaymentRepository;
use App\Modules\Payment\Domain\Repository\PaymentPreviewRepository;
use App\Modules\Payment\Domain\Service\StripeEventMapper;
use App\Modules\Payment\Domain\Service\AdyenEventMapper;
use App\Modules\Order\Domain\Event\OrderPaid;
use App\Modules\Order\Domain\Repository\OrderRepository;
use App\Modules\Shop\Domain\Repository\ShopLedgerRepository;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Event;
use App\Modules\Payment\Domain\Ledger\PostingType;
use App\Modules\Payment\Application\UseCase\Ledger\PostFeeFromStripeChargeUseCase;
use App\Modules\Payment\Domain\Ledger\Port\PostLedgerPort;
use App\Modules\Payment\Domain\Ledger\Port\PostLedgerCommand;
use App\Modules\Order\Application\UseCase\MarkOrderPaidUseCase;
use App\Modules\Item\Application\UseCase\Item\Command\DecreaseItemStockOnPaidUseCase;
use App\Modules\Order\Domain\Repository\OrderHistoryRepository;

final class HandlePaymentWebhookUseCase
{
    public function __construct(
        private PaymentQueryRepository $webhookEvents,
        private PaymentRepository $payments,
        private OrderRepository $orders,
        private ShopLedgerRepository $ledgers,
        private StripeEventMapper $mapper,
        private AdyenEventMapper $adyenMapper,
        private PostLedgerPort $port,
        private PostFeeFromStripeChargeUseCase $postFee,
        private MarkOrderPaidUseCase $markOrderPaid,
        private PaymentPreviewRepository $previews, // ✅ Adyen: preview_key 解決に必要
        private DecreaseItemStockOnPaidUseCase $decreaseStock,
        private OrderHistoryRepository $orderHistory,
    ) {}

    public function handle(HandlePaymentWebhookInput $input): void
    {
        // =========================
        // 0) 冪等ロック（reserve）
        // =========================
        $reserved = $this->safeReserve($input);

        // ✅ reserve=true 以外は「闇に落とさない」
        if ($reserved !== true) {
            // reserve=false（既に処理済み）なら何もしない（静かに終了）
            if ($reserved === false) {
                return;
            }

            // reserve=null（例外など）なら、errorとして記録だけ残す
            $this->safeComplete(
                $input,
                'error',
                null,
                null,
                'reserve_failed'
            );
            return;
        }

        // =========================
        // map はここで1回だけ（provider-aware）
        // =========================
        $domainEvent = ($input->provider === 'adyen')
            ? $this->adyenMapper->map($input)
            : $this->mapper->map($input);

        // =========================
        // complete を1回だけ呼ぶための最終状態
        // =========================
        $finalStatus = 'ok';              // ok | ignored | error
        $finalPaymentId = null;           // ?int
        $finalOrderId = null;             // ?int
        $finalErrorMessage = null;        // ?string

        // afterCommit dispatch 用
        /** @var OrderPaid|null */
        $orderPaidEvent = null;

        try {
            // payload metadata から拾う（Paymentが無い救済・監査紐付けのため）
            // ✅ Adyen は order_id を payload から直接取らない（preview_key 経由）
            $orderIdFromMeta = $this->extractOrderIdFromPayloadMeta($input);
            $paymentIdFromMeta = $this->extractPaymentIdFromPayloadMeta($input);

            // =========================
            // 3) IGNORED は「監査ログだけ残して終了」
            // =========================
            if ($domainEvent->type === DomainPaymentEventType::IGNORED) {

                // Stripeだけ fee-only を許す（Adyen payloadで落ちないようガード）
                if ($input->provider === 'stripe' && str_starts_with($input->eventType, 'charge.')) {
                    $this->handleFeeOnlyIfPossible($input, $domainEvent, $paymentIdFromMeta, $orderIdFromMeta);
                }

                // ✅ 何も処理しないなら ok ではなく ignored
                $finalStatus = 'ignored';
                $finalPaymentId = is_int($paymentIdFromMeta) ? $paymentIdFromMeta : null;
                $finalOrderId = is_int($orderIdFromMeta) ? $orderIdFromMeta : null;
                return; // finally が走るので complete は必ず呼ばれる
            }

            // =========================
            // 4) 本処理（transaction）
            // =========================
            DB::transaction(function () use (
                $input,
                $domainEvent,
                $orderIdFromMeta,
                $paymentIdFromMeta,
                &$finalStatus,
                &$finalPaymentId,
                &$finalOrderId,
                &$finalErrorMessage,
                &$orderPaidEvent
            ) {
                $isStripe = ($input->provider === 'stripe');
                $isAdyen  = ($input->provider === 'adyen');

                // -----------------------------------------
                // 4-1) Payment 探索順（R3固定）
                //  (1) provider_payment_id -> Payment
                //  (2) metadata.payment_id -> Payment
                // -----------------------------------------
                $payment = null;

                if (is_string($domainEvent->providerPaymentId) && $domainEvent->providerPaymentId !== '') {
                    $payment = $this->payments->findByProviderPaymentId($domainEvent->providerPaymentId);
                }

                if (!$payment && is_int($paymentIdFromMeta)) {
                    $payment = $this->payments->findById($paymentIdFromMeta);
                }

                // Payment が取れた場合は監査紐付けを確定
                if ($payment) {
                    $finalPaymentId = $payment->id();
                    $finalOrderId = $payment->orderId();
                } else {
                    $finalPaymentId = is_int($paymentIdFromMeta) ? $paymentIdFromMeta : null;
                    $finalOrderId = is_int($orderIdFromMeta) ? $orderIdFromMeta : null;
                }

                // -----------------------------------------
                // Stripe fee: charge.* のみ（Adyenは触らない）
                // -----------------------------------------
                if ($isStripe && $payment && str_starts_with($input->eventType, 'charge.')) {
                    $charge = $input->payload['data']['object'] ?? [];
                    $balanceTxnId = is_array($charge) ? ($charge['balance_transaction'] ?? null) : null;

                    if (is_string($balanceTxnId) && $balanceTxnId !== '') {
                        $this->postFee->handle(
                            balanceTransactionId: $balanceTxnId,
                            shopId: $payment->shopId(),
                            orderId: $payment->orderId(),
                            paymentId: $payment->id(),
                            occurredAt: $domainEvent->occurredAt,
                            meta: [
                                // ✅ 命名固定
                                'idempotency_key'     => $input->eventId,          // sha256
                                'provider_event_id'   => $input->providerEventId,  // evt_... / pspRef
                                'webhook_event_type'  => $input->eventType,

                                'provider_payment_id' => $domainEvent->providerPaymentId,
                                'charge_id'           => $charge['id'] ?? null,
                            ],
                        );
                    }
                }

                // -----------------------------------------
                // 4-2) Payment が無い救済ルート（Stripeのみ）
                //  - Adyenは merchantReference(=preview_key) 起点で処理する
                // -----------------------------------------
                if (!$payment && !$isAdyen) {

                    if ($domainEvent->type !== DomainPaymentEventType::SUCCEEDED) {
                        // ✅ 何も処理しないなら ignored に寄せる（ok禁止）
                        $finalStatus = 'ignored';
                        return;
                    }

                    if (!is_int($orderIdFromMeta)) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'stripe_order_id_missing_in_metadata';
                        return;
                    }

                    $order = $this->orders->findById($orderIdFromMeta);
                    if (!$order) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'stripe_order_not_found';
                        return;
                    }

                    $finalOrderId = $order->id();

                    if ($order->isPaid()) {
                        $finalStatus = 'ignored';
                        return;
                    }

                    $orderPaidEvent = $this->markOrderPaid->handle(
                        orderId: $order->id(),
                        paidAt: $domainEvent->occurredAt,
                    );

                    // ✅ paid遷移が起きた時だけ在庫減算（ここでは payment が無いので order 起点）
                    if ($orderPaidEvent) {
                        $this->decreaseStockSafely(
                            input: $input,
                            shopId: (int)$order->shopId(),
                            orderId: (int)$order->id(),
                            paymentId: null,
                        );
                    }

                    // Payment不在では台帳は起こさない（現状方針）
                    $finalStatus = 'ok';
                    return;
                }

                // -----------------------------------------
                // 4-3) 安全装置：metadata.order_id と Payment.orderId の一致（Stripe寄り）
                // -----------------------------------------
                if ($payment && is_int($orderIdFromMeta) && $orderIdFromMeta !== $payment->orderId()) {
                    $finalStatus = 'error';
                    $finalErrorMessage = 'metadata_order_id_mismatch';
                    return;
                }

                // -----------------------------------------
                // 4-4) Refund（Stripeのみ）
                // -----------------------------------------
                if ($isStripe && $domainEvent->type === DomainPaymentEventType::REFUND_SUCCEEDED) {

                    $meta = $domainEvent->instructions ?? [];
                    $refundId = $meta['provider_refund_id'] ?? null;

                    if (!is_string($refundId) || $refundId === '') {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'stripe_refund_id_missing';
                        return;
                    }

                    $refundAmount = $meta['refund_amount'] ?? null;
                    if (!is_numeric($refundAmount)) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'stripe_refund_amount_invalid';
                        return;
                    }
                    $refundAmount = (int)$refundAmount;
                    if ($refundAmount <= 0) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'stripe_refund_amount_non_positive';
                        return;
                    }

                    $refundCurrency = $meta['currency'] ?? $payment->currency();
                    $refundCurrency = is_string($refundCurrency) && $refundCurrency !== ''
                        ? $refundCurrency
                        : $payment->currency();

                    if ($this->ledgers->existsRefundByProviderRefundId('stripe', $refundId)) {
                        $finalStatus = 'ignored';
                        return;
                    }

                    $this->ledgers->recordRefund(
                        shopId: $payment->shopId(),
                        amount: $refundAmount,
                        currency: $refundCurrency,
                        orderId: $payment->orderId(),
                        paymentId: $payment->id(),
                        provider: 'stripe',
                        providerRefundId: $refundId,
                        reason: $meta['reason'] ?? null,
                        occurredAt: $domainEvent->occurredAt,
                    );

                    $sourceId = $refundId . ':' . PostingType::REFUND;

                    $this->port->post(new PostLedgerCommand(
                        source_provider: 'stripe',
                        source_event_id: $sourceId,
                        shop_id: $payment->shopId(),
                        order_id: $payment->orderId(),
                        payment_id: $payment->id(),
                        posting_type: PostingType::REFUND,
                        amount: $refundAmount,
                        currency: $refundCurrency,
                        occurred_at: $domainEvent->occurredAt->format('Y-m-d H:i:s'),
                        meta: [
                            // ✅ 命名固定
                            'idempotency_key'     => $input->eventId,
                            'provider_event_id'   => $input->providerEventId,
                            'webhook_event_type'  => $input->eventType,

                            'provider_payment_id' => $domainEvent->providerPaymentId,
                            'provider_refund_id'  => $refundId,
                            'refund_amount'       => $refundAmount,
                        ],
                        replay: false,
                    ));

                    $finalStatus = 'ok';
                    return;
                }

                // -----------------------------------------
                // 4-5) SUCCEEDED / FAILED / REQUIRES_ACTION
                // -----------------------------------------

                // ===== Adyen (MVP) =====
                if ($isAdyen) {

                    // 現状MVPでは AUTHORISATION を最初の確定イベントとして扱う
                    if ($input->eventType !== 'AUTHORISATION') {
                        // ✅ ok にしない
                        $finalStatus = 'ignored';
                        return;
                    }

                    // ✅ merchantReference = preview_key(UUID)
                    $previewKey = $this->extractPreviewKeyFromAdyen($input);
                    if (!is_string($previewKey) || $previewKey === '') {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_preview_key_missing';
                        return;
                    }

                    // ✅ committed preview を解決（ここが正統ルート）
                    $preview = $this->previews->findCommittedByKey($previewKey);
                    if (!$preview) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_preview_not_committed';
                        return;
                    }

                    $orderId = is_numeric($preview->order_id ?? null) ? (int)$preview->order_id : null;
                    $paymentId = is_numeric($preview->payment_id ?? null) ? (int)$preview->payment_id : null;

                    if (!$orderId || !$paymentId) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_preview_missing_order_or_payment';
                        return;
                    }

                    $order = $this->orders->findById($orderId);
                    if (!$order) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_order_not_found';
                        return;
                    }

                    $finalOrderId = $order->id();
                    $finalPaymentId = $paymentId;

                    // Payment を解決（最優先 payment_id）
                    $payment = $this->payments->findById($paymentId);
                    if (!$payment) {
                        // 念のため救済（通常ここには来ないはず）
                        $payment = $this->payments->findLatestByOrderId($orderId);
                    }
                    if (!$payment) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_payment_not_found';
                        return;
                    }

                    // ✅ amount/currency チェック（不一致なら絶対に paid にしない）
                    $nriAmount   = $input->payload['amount']['value'] ?? null;
                    $nriCurrency = $input->payload['amount']['currency'] ?? null;

                    $webhookAmount   = is_numeric($nriAmount) ? (int)$nriAmount : null;
                    $webhookCurrency = is_string($nriCurrency) ? strtoupper($nriCurrency) : null;

                    $expectedAmount   = $payment->amount();
                    $expectedCurrency = strtoupper($payment->currency());

                    if ($webhookAmount === null || $webhookCurrency === null) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_amount_currency_missing';
                        return;
                    }

                    if ($webhookAmount !== $expectedAmount || $webhookCurrency !== $expectedCurrency) {
                        $finalStatus = 'error';
                        $finalErrorMessage = 'adyen_amount_currency_mismatch';
                        return;
                    }

                    if ($domainEvent->type === DomainPaymentEventType::FAILED) {
                        $this->payments->save(
                            $payment->markFailed([
                                'reason' => $domainEvent->reason ?? 'adyen_failed'
                            ])
                        );

                        $finalStatus = 'ok';
                        return;
                    }

                    if ($domainEvent->type === DomainPaymentEventType::SUCCEEDED) {

                        // Payment succeeded（pspReference を provider_payment_id に確定させる）
                        $payment = $payment
                            ->withProviderPayment($domainEvent->providerPaymentId)
                            ->markSucceeded();

                        $this->payments->save($payment);

                        // Order paid（冪等は MarkOrderPaidUseCase で担保）
                        $orderPaidEvent = $this->markOrderPaid->handle(
                            orderId: $order->id(),
                            paidAt: $domainEvent->occurredAt,
                        );

                        // ✅ paid遷移が起きた時だけ在庫減算（payment があるので payment 起点）
                        if ($orderPaidEvent) {
                            $this->decreaseStockSafely(
                                input: $input,
                                shopId: (int)$payment->shopId(),
                                orderId: (int)$order->id(),
                                paymentId: (int)$payment->id(),
                            );
                        }

                        // ✅ ledger_postings に SALE
                        $pspRef = (string)($input->payload['pspReference'] ?? '');
                        $providerPaymentId = is_string($domainEvent->providerPaymentId) ? $domainEvent->providerPaymentId : '';

                        $sourceEventIdBase = $pspRef !== '' ? $pspRef : $providerPaymentId;
                        if ($sourceEventIdBase === '') {
                            $sourceEventIdBase = $input->eventId;
                        }

                        $this->port->post(new PostLedgerCommand(
                            source_provider: 'adyen',
                            source_event_id: $sourceEventIdBase . ':' . PostingType::SALE,
                            shop_id: $payment->shopId(),
                            order_id: $order->id(),
                            payment_id: $payment->id(),
                            posting_type: PostingType::SALE,
                            amount: $webhookAmount,
                            currency: $webhookCurrency,
                            occurred_at: $domainEvent->occurredAt->format('Y-m-d H:i:s'),
                            meta: [
                                // ✅ 命名固定
                                'idempotency_key'     => $input->eventId,
                                'provider_event_id'   => $input->providerEventId, // pspReference
                                'webhook_event_type'  => $input->eventType,

                                'provider_payment_id' => $providerPaymentId !== '' ? $providerPaymentId : null,
                                'psp_reference'       => $pspRef !== '' ? $pspRef : null,
                                'merchant_reference'  => $previewKey, // ✅ preview_key
                                'success'             => $input->payload['success'] ?? null,
                            ],
                            replay: false,
                        ));

                        $finalStatus = 'ok';
                        return;
                    }

                    // SUCCEEDED/FAILED 以外でここまで来たら ignored
                    $finalStatus = 'ignored';
                    return;
                }

                // ===== Stripe existing logic =====
                if ($input->eventType !== 'payment_intent.succeeded') {
                    $finalStatus = 'ignored';
                    return;
                }

                if ($domainEvent->type === DomainPaymentEventType::FAILED) {
                    if ($payment) {
                        $this->payments->save($payment->markFailed(['reason' => $domainEvent->reason]));
                    }
                    $finalStatus = 'ok';
                    return;
                }

                if ($domainEvent->type === DomainPaymentEventType::REQUIRES_ACTION) {
                    if ($payment) {
                        $this->payments->save($payment->markRequiresAction());
                    }
                    $finalStatus = 'ok';
                    return;
                }

                if ($domainEvent->type !== DomainPaymentEventType::SUCCEEDED) {
                    $finalStatus = 'ignored';
                    return;
                }

                // SUCCEEDED ここから先で台帳を起こす
                if (!$payment) {
                    $finalStatus = 'error';
                    $finalErrorMessage = 'stripe_payment_not_found';
                    return;
                }

                // Payment succeeded
                $payment = $payment->markSucceeded();
                $this->payments->save($payment);

                // Order paid（冪等は MarkOrderPaidUseCase で担保）
                $orderPaidEvent = $this->markOrderPaid->handle(
                    orderId: $payment->orderId(),
                    paidAt: $domainEvent->occurredAt,
                );

                // ✅ paid遷移が起きた時だけ在庫減算
                if ($orderPaidEvent) {
                    $this->decreaseStockSafely(
                        input: $input,
                        shopId: (int)$payment->shopId(),
                        orderId: (int)$payment->orderId(),
                        paymentId: (int)$payment->id(),
                    );
                }

                // Stripeの売上台帳（shop_ledgers）
                $this->ledgers->recordSale(
                    shopId: $payment->shopId(),
                    amount: $payment->amount(),
                    currency: $payment->currency(),
                    orderId: $payment->orderId(),
                    paymentId: $payment->id(),
                    occurredAt: $domainEvent->occurredAt,
                );

                // Stripeの売上posting（ledger_postings）
                $this->port->post(new PostLedgerCommand(
                    source_provider: 'stripe',
                    source_event_id: $domainEvent->providerPaymentId . ':' . PostingType::SALE,
                    shop_id: $payment->shopId(),
                    order_id: $payment->orderId(),
                    payment_id: $payment->id(),
                    posting_type: PostingType::SALE,
                    amount: $payment->amount(),
                    currency: $payment->currency(),
                    occurred_at: $domainEvent->occurredAt->format('Y-m-d H:i:s'),
                    meta: [
                        // ✅ 命名固定
                        'idempotency_key'     => $input->eventId,
                        'provider_event_id'   => $input->providerEventId, // evt_...
                        'webhook_event_type'  => $input->eventType,

                        'provider_payment_id' => $domainEvent->providerPaymentId,
                    ],
                    replay: false,
                ));

                $finalStatus = 'ok';
                return;
            });

            // ✅ transaction が無事に完走した時だけ ok
            // （中で ignored/error に落としている場合はそれを尊重する）
            if ($finalStatus !== 'ignored' && $finalStatus !== 'error') {
                $finalStatus = 'ok';
            }

        } catch (\Throwable $e) {
            // PSP に 500 を返さない方針（swallow）
            $finalStatus = 'error';
            $finalErrorMessage = $e->getMessage();
            return;

        } finally {
            // complete（必ず1回だけ）
            $this->safeComplete(
                $input,
                $finalStatus,
                $finalPaymentId,
                $finalOrderId,
                $finalErrorMessage
            );

            // afterCommit dispatch
            if ($orderPaidEvent) {
                DB::afterCommit(fn () => Event::dispatch($orderPaidEvent));
            }
        }
    }

    /**
     * Stripe: payload.data.object.metadata.order_id
     * Adyen: order_id を payload からは取らない（preview_key 経由に固定）
     */
    private function extractOrderIdFromPayloadMeta(HandlePaymentWebhookInput $input): ?int
    {
        // ✅ Adyen は order_id を payload から直接取らない
        if ($input->provider === 'adyen') {
            return null;
        }

        // Stripe existing
        $payload = $input->payload;
        $object  = $payload['data']['object'] ?? [];

        if (isset($object['metadata']) && is_array($object['metadata'])) {
            $oid = $object['metadata']['order_id'] ?? null;
            if (is_numeric($oid)) {
                return (int)$oid;
            }
        }

        return null;
    }

    /**
     * Adyen: merchantReference = preview_key(uuid)
     */
    private function extractPreviewKeyFromAdyen(HandlePaymentWebhookInput $input): ?string
    {
        if ($input->provider !== 'adyen') return null;

        $nri = $input->payload;
        if (!is_array($nri)) return null;

        $mr = $nri['merchantReference'] ?? null;
        if (!is_string($mr) || $mr === '') return null;

        return $mr; // ✅ preview_key(UUID)
    }

    /**
     * Stripe payload 内の metadata.payment_id を拾う（Payment 不在救済の第一候補）
     * AdyenはMVPでは使わないので null
     */
    private function extractPaymentIdFromPayloadMeta(HandlePaymentWebhookInput $input): ?int
    {
        if ($input->provider === 'adyen') {
            return null;
        }

        $payload = $input->payload;
        $object  = $payload['data']['object'] ?? [];

        if (isset($object['metadata']) && is_array($object['metadata'])) {
            $pid = $object['metadata']['payment_id'] ?? null;
            if (is_numeric($pid)) {
                return (int)$pid;
            }
        }

        return null;
    }

    private function safeReserve(HandlePaymentWebhookInput $input): bool|null
    {
        try {
            return $this->webhookEvents->reserve(
                $input->provider,
                $input->eventId,         // sha256 hex64（冪等キー）
                $input->providerEventId, // evt_... / pspReference（外部ID）
                $input->eventType,
                $input->payloadHash
            );
        } catch (\Throwable) {
            return null;
        }
    }

    private function safeComplete(
        HandlePaymentWebhookInput $input,
        string $status,
        ?int $paymentId,
        ?int $orderId,
        ?string $errorMessage,
    ): void {
        try {
            $this->webhookEvents->complete(
                $input->provider,
                $input->eventId,  // sha256 hex64（冪等キー）
                $status,
                $paymentId,
                $orderId,
                $errorMessage,
                null // errorCode（必要ならここに入れる）
            );
        } catch (\Throwable) {
            // swallow
        }
    }

    /**
     * Stripe only
     */
    private function handleFeeOnlyIfPossible(
        HandlePaymentWebhookInput $input,
        $domainEvent,
        ?int $paymentIdFromMeta,
        ?int $orderIdFromMeta,
    ): void {
        if ($input->provider !== 'stripe') {
            return;
        }

        if (!str_starts_with($input->eventType, 'charge.')) {
            return;
        }

        $charge = $input->payload['data']['object'] ?? [];
        if (!is_array($charge)) {
            return;
        }

        $balanceTxnId = $charge['balance_transaction'] ?? null;
        if (!is_string($balanceTxnId) || $balanceTxnId === '') {
            return;
        }

        $payment = null;

        if (is_int($paymentIdFromMeta)) {
            $payment = $this->payments->findById($paymentIdFromMeta);
        }

        if (!$payment) {
            $piId = $charge['payment_intent'] ?? null;
            if (is_string($piId) && $piId !== '') {
                $payment = $this->payments->findByProviderPaymentId($piId);
            }
        }

        if (!$payment) {
            return;
        }

        $this->postFee->handle(
            balanceTransactionId: $balanceTxnId,
            shopId: $payment->shopId(),
            orderId: $orderIdFromMeta ?? $payment->orderId(),
            paymentId: $payment->id(),
            occurredAt: $domainEvent->occurredAt,
            meta: [
                // ✅ 命名固定
                'idempotency_key'     => $input->eventId,
                'provider_event_id'   => $input->providerEventId,
                'webhook_event_type'  => $input->eventType,

                'provider_payment_id' => $domainEvent->providerPaymentId,
                'charge_id'           => $charge['id'] ?? null,
            ],
        );
    }

    /**
     * ✅ 在庫減算は「失敗しても throw しない」＋ order_events に必ず残す
     * - rows は order_items のスナップショットを唯一の真実として使う
     */
    private function decreaseStockSafely(
        HandlePaymentWebhookInput $input,
        int $shopId,
        int $orderId,
        ?int $paymentId,
    ): void {
        $rows = DB::table('order_items')
            ->where('order_id', $orderId)
            ->select(['item_id', 'quantity'])
            ->get()
            ->map(fn($r) => ['item_id' => (int)$r->item_id, 'quantity' => (int)$r->quantity])
            ->all();

        try {
            $this->decreaseStock->handle($shopId, $rows);

            $this->orderHistory->addEvent($orderId, 'inventory_decreased', [
                'shop_id' => $shopId,
                'payment_id' => $paymentId,
                'provider' => $input->provider,

                // ✅ 命名固定
                'idempotency_key'     => $input->eventId,
                'provider_event_id'   => $input->providerEventId,
                'webhook_event_type'  => $input->eventType,

                'items' => $rows,
            ]);
        } catch (\Throwable $e) {
            // ★ ここで throw しない（PSPに500返さない / paidは維持）
            $this->orderHistory->addEvent($orderId, 'inventory_decrease_failed', [
                'shop_id' => $shopId,
                'payment_id' => $paymentId,
                'provider' => $input->provider,

                // ✅ 命名固定
                'idempotency_key'     => $input->eventId,
                'provider_event_id'   => $input->providerEventId,
                'webhook_event_type'  => $input->eventType,

                'items' => $rows,
                'error' => [
                    'type' => get_class($e),
                    'message' => $e->getMessage(),
                ],
            ]);
        }
    }

    private function buildEventId(string $provider, string $providerEventId, string $eventType): string
    {
        return hash('sha256', $provider . '|' . $providerEventId . '|' . $eventType);
    }
}