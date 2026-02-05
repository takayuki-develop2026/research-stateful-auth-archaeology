<?php

namespace App\Modules\Payment\Application\UseCase;

use App\Modules\Order\Application\Dto\CreateOrderInput;
use App\Modules\Order\Application\UseCase\CreateOrderUseCase;
use App\Modules\Order\Application\UseCase\ConfirmOrderAddressUseCase;
use App\Modules\Order\Application\UseCase\ConfirmOrderUseCase;
use App\Modules\Payment\Application\Dto\AdyenCommitInput;
use App\Modules\Payment\Domain\Entity\Payment;
use App\Modules\Payment\Domain\Enum\PaymentMethod;
use App\Modules\Payment\Domain\Enum\PaymentProvider;
use App\Modules\Payment\Domain\Repository\PaymentRepository;
use App\Modules\Payment\Domain\Repository\PaymentPreviewRepository;
use Illuminate\Support\Facades\DB;

final class CommitAdyenCheckoutUseCase
{
    public function __construct(
        private PaymentPreviewRepository $previews,
        private CreateOrderUseCase $createOrder,
        private ConfirmOrderAddressUseCase $confirmAddress,
        private ConfirmOrderUseCase $confirmOrder,
        private PaymentRepository $payments,
    ) {}

    /**
     * commitは「注文を確定（住所含む）＋Paymentレコード作成」まで。
     * Adyenの実決済は、すでに表示中のDrop-in(session)が実行する。
     */
    public function handle(AdyenCommitInput $in): array
    {
        return DB::transaction(function () use ($in) {

            $preview = $this->previews->findActiveByKey($in->previewKey);
            if (! $preview) {
                throw new \DomainException('preview_key is invalid or expired');
            }

            if ((int)$preview->user_id !== $in->userId) {
                throw new \DomainException('preview_key forbidden');
            }

            if ((int)$preview->shop_id !== $in->shopId) {
                throw new \DomainException('preview_key shop mismatch');
            }

            // 金額一致（改ざん対策）
            // ここは最低限：itemsから合計を算出し preview.amount と一致させる
            $calcTotal = 0;
            $currency = null;
            foreach ($in->items as $row) {
                $q = (int)($row['quantity'] ?? 1);
                $amt = (int)$row['price_amount'];
                $cur = (string)$row['price_currency'];
                $currency ??= $cur;
                if ($cur !== $currency) throw new \DomainException('Mixed currency not supported');
                $calcTotal += $amt * $q;
            }

            if ((int)$preview->amount !== $calcTotal) {
                throw new \DomainException('amount mismatch');
            }

            if (strtoupper((string)$preview->currency) !== strtoupper((string)$currency)) {
                throw new \DomainException('currency mismatch');
            }

            // 1) Order作成（あなたのCreateOrderUseCaseに完全一致）
            $orderOut = $this->createOrder->handle(new CreateOrderInput(
                shopId: $in->shopId,
                userId: $in->userId,
                items: $in->items,
                meta: $in->meta,
            ));
            $orderId = (int)$orderOut->orderId;

            // 2) 住所確定（あなたのConfirmOrderAddressUseCaseそのまま）
            $this->confirmAddress->handle(
                orderId: $orderId,
                userId: $in->userId,
                addressId: $in->addressId
            );

            // 3) confirm（validate-only）
            $this->confirmOrder->handle(
                orderId: $orderId,
                userId: $in->userId
            );

            // 4) Paymentレコード作成（gatewayは呼ばない）
            $payment = Payment::initiate(
                orderId: $orderId,
                shopId: $in->shopId,
                userId: $in->userId,
                provider: PaymentProvider::ADYEN,
                method: PaymentMethod::CARD,
                amount: $calcTotal,
                currency: strtoupper((string)$currency),
                meta: [
                    'preview_key' => $in->previewKey,
                    'adyen_session_id' => (string)$preview->session_id,
                ]
            );

            // provider_payment_id は session_id を入れてOK（後で webhook で pspReference に上書きしてもOK）
            $payment = $payment->withProviderPayment((string)$preview->session_id);

            $payment = $this->payments->save($payment);
            $paymentId = (int)$payment->id();

            // 5) previewを committed にする（Webhookでorder/paymentを引ける）
            $this->previews->markCommitted($in->previewKey, $orderId, $paymentId);

            return [
                'order_id' => $orderId,
                'payment_id' => $paymentId,
            ];
        });
    }
}