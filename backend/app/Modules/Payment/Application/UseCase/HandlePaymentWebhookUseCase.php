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
    ) {}

    public function handle(HandlePaymentWebhookInput $input): void
    {
        // =========================
        // 0) 冪等ロック（reserve）
        // =========================
        $reserved = $this->safeReserve($input);
        if ($reserved !== true) {
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
                                'provider_payment_id' => $domainEvent->providerPaymentId,
                                'charge_id' => $charge['id'] ?? null,
                                'webhook_event_type' => $input->eventType,
                                'webhook_event_id' => $input->eventId,
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
                        return;
                    }

                    if (!is_int($orderIdFromMeta)) {
                        return;
                    }

                    $order = $this->orders->findById($orderIdFromMeta);
                    if (!$order) {
                        return;
                    }

                    $finalOrderId = $order->id();

                    if ($order->isPaid()) {
                        return;
                    }

                    $orderPaidEvent = $this->markOrderPaid->handle(
                        orderId: $order->id(),
                        paidAt: $domainEvent->occurredAt,
                    );

                    // Payment不在では台帳は起こさない（現状方針）
                    return;
                }

                // -----------------------------------------
                // 4-3) 安全装置：metadata.order_id と Payment.orderId の一致（Stripe寄り）
                // -----------------------------------------
                if ($payment && is_int($orderIdFromMeta) && $orderIdFromMeta !== $payment->orderId()) {
                    return;
                }

                // -----------------------------------------
                // 4-4) Refund（Stripeのみ）
                // -----------------------------------------
                if ($isStripe && $domainEvent->type === DomainPaymentEventType::REFUND_SUCCEEDED) {

                    $meta = $domainEvent->instructions ?? [];
                    $refundId = $meta['provider_refund_id'] ?? null;

                    if (!is_string($refundId) || $refundId === '') {
                        return;
                    }

                    $refundAmount = $meta['refund_amount'] ?? null;
                    if (!is_numeric($refundAmount)) {
                        return;
                    }
                    $refundAmount = (int)$refundAmount;
                    if ($refundAmount <= 0) {
                        return;
                    }

                    $refundCurrency = $meta['currency'] ?? $payment->currency();
                    $refundCurrency = is_string($refundCurrency) && $refundCurrency !== ''
                        ? $refundCurrency
                        : $payment->currency();

                    if ($this->ledgers->existsRefundByProviderRefundId('stripe', $refundId)) {
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
                            'provider_payment_id' => $domainEvent->providerPaymentId,
                            'provider_refund_id'  => $refundId,
                            'refund_amount'       => $refundAmount,
                            'webhook_event_type'  => $input->eventType,
                            'webhook_event_id'    => $input->eventId,
                        ],
                        replay: false,
                    ));

                    return;
                }

                // -----------------------------------------
                // 4-5) SUCCEEDED / FAILED / REQUIRES_ACTION
                // -----------------------------------------

                // ===== Adyen (MVP) =====
                if ($isAdyen) {

                    // 現状MVPでは AUTHORISATION を最初の確定イベントとして扱う
                    if ($input->eventType !== 'AUTHORISATION') {
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
                                'provider_payment_id' => $providerPaymentId !== '' ? $providerPaymentId : null,
                                'psp_reference'       => $pspRef !== '' ? $pspRef : null,
                                'merchant_reference'  => $previewKey, // ✅ preview_key
                                'webhook_event_type'  => $input->eventType,
                                'webhook_event_id'    => $input->eventId,
                                'success'             => $input->payload['success'] ?? null,
                            ],
                            replay: false,
                        ));

                        return;
                    }

                    return;
                }

                // ===== Stripe existing logic =====
                if ($input->eventType !== 'payment_intent.succeeded') {
                    return;
                }

                if ($domainEvent->type === DomainPaymentEventType::FAILED) {
                    if ($payment) {
                        $this->payments->save($payment->markFailed(['reason' => $domainEvent->reason]));
                    }
                    return;
                }

                if ($domainEvent->type === DomainPaymentEventType::REQUIRES_ACTION) {
                    if ($payment) {
                        $this->payments->save($payment->markRequiresAction());
                    }
                    return;
                }

                if ($domainEvent->type !== DomainPaymentEventType::SUCCEEDED) {
                    return;
                }

                // SUCCEEDED ここから先で台帳を起こす
                if (!$payment) {
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
                        'provider_payment_id' => $domainEvent->providerPaymentId,
                        'webhook_event_type' => $input->eventType,
                        'webhook_event_id' => $input->eventId,
                    ],
                    replay: false,
                ));

                return;
            });

            // 正常終了
            $finalStatus = 'ok';

        } catch (\Throwable $e) {
            $finalStatus = 'error';
            $finalErrorMessage = $e->getMessage();
            throw $e;

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
                $input->eventId,
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
                $input->eventId,
                $status,
                $paymentId,
                $orderId,
                $errorMessage,
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
                'provider_payment_id' => $domainEvent->providerPaymentId,
                'charge_id' => $charge['id'] ?? null,
                'webhook_event_type' => $input->eventType,
                'webhook_event_id' => $input->eventId,
            ],
        );
    }
}