<?php

namespace App\Modules\Payment\Application\UseCase;

use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Domain\Enum\PaymentStatus;
use App\Modules\Payment\Domain\Event\DomainPaymentEventType;
use App\Modules\Payment\Domain\Repository\PaymentQueryRepository;
use App\Modules\Payment\Domain\Repository\PaymentRepository;
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
        private PaymentQueryRepository $webhookEvents, // reserve/complete の正（processed正統）
        private PaymentRepository $payments,
        private OrderRepository $orders,
        private ShopLedgerRepository $ledgers,
        private StripeEventMapper $mapper,
        private AdyenEventMapper $adyenMapper,
        private PostLedgerPort $port,
        private PostFeeFromStripeChargeUseCase $postFee,
        private MarkOrderPaidUseCase $markOrderPaid,
    ) {
    }

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
                &$finalPaymentId,
                &$finalOrderId,
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

                // providerPaymentId は providerごとに意味が違うが、まずは共通で引く
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
                    // Payment が無い場合でも、メタから取れるなら監査紐付け
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
                //  - Adyenは 4-5 で処理する（merchantReference起点）
                // -----------------------------------------
                if (!$payment && !$isAdyen) {

                    // SUCCEEDED 以外は何もしない（監査ログは complete で残る）
                    if ($domainEvent->type !== DomainPaymentEventType::SUCCEEDED) {
                        return;
                    }

                    // order_id が取れないなら何もしない
                    if (!is_int($orderIdFromMeta)) {
                        return;
                    }

                    $order = $this->orders->findById($orderIdFromMeta);
                    if (!$order) {
                        return;
                    }

                    $finalOrderId = $order->id();

                    // すでに paid なら何もしない（冪等）
                    if ($order->isPaid()) {
                        return;
                    }

                    // ✅ Order を paid に進める（occurredAt を正）
                    $orderPaidEvent = $this->markOrderPaid->handle(
                        orderId: $order->id(),
                        paidAt: $domainEvent->occurredAt,
                    );

                    // Ledger は “Payment不在” では原則記録しない（現状方針のまま）
                    return;
                }

                // -----------------------------------------
                // 4-3) 安全装置：metadata.order_id と Payment.orderId の一致（Stripe寄り）
                // -----------------------------------------
                if ($payment && is_int($orderIdFromMeta) && $orderIdFromMeta !== $payment->orderId()) {
                    // 別注文への誤紐付け可能性 => 触らない（監査ログだけ残す）
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

                    // 冪等（shop_ledgers側）
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

                    if ($input->eventType !== 'AUTHORISATION') {
                        return;
                    }

                    // order_id は merchantReference（numeric前提）
                    if (!is_int($orderIdFromMeta)) {
                        return;
                    }

                    // Paymentが取れてなければ orderId で救済
                    if (!$payment) {
                        $payment = $this->payments->findLatestByOrderId($orderIdFromMeta);
                    }

                    $order = $this->orders->findById($orderIdFromMeta);
                    if (!$order) {
                        return;
                    }

                    $finalOrderId = $order->id();
                    if ($payment) {
                        $finalPaymentId = $payment->id();
                    }

                    if ($domainEvent->type === DomainPaymentEventType::FAILED) {
                        if ($payment) {
                            $this->payments->save(
                                $payment->markFailed([
                                    'reason' => $domainEvent->reason ?? 'adyen_failed'
                                ])
                            );
                        }
                        return;
                    }

                    if ($domainEvent->type === DomainPaymentEventType::SUCCEEDED) {
                        if ($payment) {
                            $payment = $payment->withProviderPayment($domainEvent->providerPaymentId)
                                               ->markSucceeded();
                            $this->payments->save($payment);
                        }

                        $orderPaidEvent = $this->markOrderPaid->handle(
                            orderId: $order->id(),
                            paidAt: $domainEvent->occurredAt,
                        );
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

                // Stripeの売上台帳
                $this->ledgers->recordSale(
                    shopId: $payment->shopId(),
                    amount: $payment->amount(),
                    currency: $payment->currency(),
                    orderId: $payment->orderId(),
                    paymentId: $payment->id(),
                    occurredAt: $domainEvent->occurredAt,
                );

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
     * Adyen: merchantReference（数値）= order_id
     * Stripe: payload.data.object.metadata.order_id
     */
    private function extractOrderIdFromPayloadMeta(HandlePaymentWebhookInput $input): ?int
    {
        if ($input->provider === 'adyen') {
            $nri = $input->payload;
            if (is_array($nri)) {
                $mr = $nri['merchantReference'] ?? null;
                if (is_string($mr) && is_numeric($mr)) {
                    return (int)$mr;
                }
            }
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