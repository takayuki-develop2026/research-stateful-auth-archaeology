<?php

namespace App\Modules\Payment\Application\UseCase;

use App\Modules\Order\Domain\Repository\OrderRepository;
use App\Modules\Order\Domain\Enum\OrderStatus;
use App\Modules\Payment\Application\Dto\StartPaymentInput;
use App\Modules\Payment\Application\Dto\StartPaymentOutput;
use App\Modules\Payment\Domain\Entity\Payment;
use App\Modules\Payment\Domain\Enum\PaymentMethod;
use App\Modules\Payment\Domain\Enum\PaymentProvider;
use App\Modules\Payment\Domain\Repository\PaymentRepository;
use App\Modules\Payment\Domain\Port\PaymentGatewayPort;
use Illuminate\Support\Facades\DB;

use App\Modules\Payment\Infrastructure\Gateway\StripePaymentGateway;
use App\Modules\Payment\Infrastructure\Gateway\AdyenPaymentGateway;

final class StartPaymentUseCase
{
    public function __construct(
        private OrderRepository $orders,
        private PaymentRepository $payments,
        // private PaymentGatewayPort $gateway, // ← なくてもOK（残してもOKだが使わない）
    ) {}

    public function handle(StartPaymentInput $input, int $userId): StartPaymentOutput
    {
        return DB::transaction(function () use ($input, $userId) {

            $order = $this->orders->findById($input->orderId);
            if (! $order) throw new \RuntimeException('Order not found');

            if ((int)$order->userId() !== $userId) throw new \DomainException('Forbidden');
            if ($order->status() !== OrderStatus::PENDING_PAYMENT) throw new \DomainException('Order is not payable');
            if ($order->shippingAddress() === null) throw new \DomainException('Shipping address must be confirmed before payment.');

            $method = PaymentMethod::from($input->method);

            // ✅ 1) provider を shop 設定優先で決定（なければ env fallback）
            $provider = $this->resolveProviderByShop($order->shopId());

            // ✅ 2) provider に応じて gateway を動的に解決
            $gateway = $this->resolveGateway($provider);

            \Log::info('[🔥StartPayment] provider resolved', [
                'shop_id' => $order->shopId(),
                'provider_value' => $provider->value,
                'gateway_class' => get_class($gateway),
                'route' => request()->path(),
            ]);

            // ✅ Adyen MVP制限（必要なら）
            if ($provider === PaymentProvider::ADYEN && $method !== PaymentMethod::CARD) {
                throw new \DomainException('Adyen MVP supports card only');
            }

            // Payment 先に作る（id確定）
            $payment = Payment::initiate(
                orderId: $order->id(),
                shopId: $order->shopId(),
                userId: $order->userId(),
                provider: $provider,
                method: $method,
                amount: $order->totalAmount(),
                currency: $order->currency(),
            );

            $payment = $this->payments->save($payment);

            $ctx = [
                'order_id'   => $order->id(),
                'payment_id' => $payment->id(),
                'user_id'    => $order->userId(),
                'shop_id'    => $order->shopId(),
                'payer_name' => '購入者-' . $order->userId(),
            ];

            // Stripe
            if ($provider === PaymentProvider::STRIPE) {

                $res = $gateway->createIntent(
                    method: $method,
                    amount: $order->totalAmount(),
                    currency: $order->currency(),
                    context: $ctx
                );

                if (empty($res['provider_payment_id'])) {
                    throw new \RuntimeException('provider_payment_id missing from gateway response');
                }

                $payment = $payment->withProviderPayment($res['provider_payment_id']);

                if (($res['requires_action'] ?? false) === true) {
                    $payment = $payment->markRequiresAction([
                        'gateway_status' => $res['status'] ?? null,
                    ]);
                }

                if (!empty($res['instructions'])) {
                    $payment = $payment->withInstructions($res['instructions']);
                }

                $payment = $this->payments->save($payment);

                return new StartPaymentOutput(
                    provider: $provider->value,
                    paymentId: $payment->id(),
                    status: $payment->status()->value,
                    providerPaymentId: $payment->providerPaymentId(),

                    clientSecret: $res['client_secret'] ?? null,

                    sessionId: null,
                    sessionData: null,
                    clientKey: null,
                    environment: null,

                    instructions: $res['instructions'] ?? null,
                );
            }

            // Adyen Sessions + Drop-in
            $res = $gateway->createSession(
                method: $method,
                amount: $order->totalAmount(),
                currency: $order->currency(),
                context: $ctx
            );

            if (empty($res['session_id']) || empty($res['session_data'])) {
                throw new \RuntimeException('Adyen session fields missing from gateway response');
            }

            // provider_payment_id があるなら保存（実装次第で session_id を入れてもOK）
            if (!empty($res['provider_payment_id'])) {
                $payment = $payment->withProviderPayment($res['provider_payment_id']);
                $payment = $this->payments->save($payment);
            }

            return new StartPaymentOutput(
                provider: $provider->value,
                paymentId: $payment->id(),
                status: $payment->status()->value,
                providerPaymentId: $payment->providerPaymentId(),

                clientSecret: null,

                sessionId: (string)$res['session_id'],
                sessionData: (string)$res['session_data'],
                clientKey: !empty($res['client_key']) ? (string)$res['client_key'] : null,
                environment: !empty($res['environment']) ? (string)$res['environment'] : 'test',

                instructions: null,
            );
        });
    }

    private function resolveProviderByShop(int $shopId): PaymentProvider
    {
        // shops.payment_provider を優先
        $v = DB::table('shops')->where('id', $shopId)->value('payment_provider');

        if (is_string($v) && $v !== '') {
            // DBに不正値が入っていたら例外にせずfallback（運用事故耐性）
            try {
                return PaymentProvider::from($v);
            } catch (\ValueError) {
                \Log::warning('[🔥StartPayment] invalid shop payment_provider, fallback to env', [
                    'shop_id' => $shopId,
                    'value' => $v,
                ]);
            }
        }

        // fallback: env
        $env = (string) env('PAYMENT_PROVIDER', PaymentProvider::STRIPE->value);
        try {
            return PaymentProvider::from($env);
        } catch (\ValueError) {
            return PaymentProvider::STRIPE;
        }
    }

    private function resolveGateway(PaymentProvider $provider): PaymentGatewayPort
    {
        // PaymentGatewayPort を「shopごとに」切り替えるため、ここで具象を make する
        return match ($provider) {
            PaymentProvider::ADYEN  => app()->make(AdyenPaymentGateway::class),
            default                => app()->make(StripePaymentGateway::class),
        };
    }
}