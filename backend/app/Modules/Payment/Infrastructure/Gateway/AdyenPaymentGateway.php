<?php

namespace App\Modules\Payment\Infrastructure\Gateway;

use App\Modules\Payment\Domain\Enum\PaymentMethod;
use App\Modules\Payment\Domain\Port\PaymentGatewayPort;
use Illuminate\Support\Facades\Http;

final class AdyenPaymentGateway implements PaymentGatewayPort
{
    public function createIntent(PaymentMethod $method, int $amount, string $currency, array $context): array
    {
        throw new \LogicException('Adyen does not use createIntent; use createSession()');
    }

    public function createOneClickIntent(string $providerCustomerId, string $providerPaymentMethodId, int $amount, string $currency, array $context): array
    {
        throw new \LogicException('Adyen one-click is out of scope for MVP');
    }

    public function createSession(PaymentMethod $method, int $amount, string $currency, array $context): array
    {
        if ($method !== PaymentMethod::CARD) {
            throw new \InvalidArgumentException('Adyen MVP supports card only');
        }

        $apiKey = (string) config('services.adyen.api_key');
        $merchantAccount = (string) config('services.adyen.merchant_account');
        $environment = (string) config('services.adyen.environment', 'test'); // test|live
        $baseUrl = (string) config('services.adyen.checkout_base_url'); // 例: https://checkout-test.adyen.com

        // ✅ 重要：webhookで照合できるよう merchantReference に payment_id を埋める
        $paymentId = (int) ($context['payment_id'] ?? 0);
        $orderId   = (int) ($context['order_id'] ?? 0);

        if ($paymentId <= 0 || $orderId <= 0) {
            throw new \RuntimeException('payment_id/order_id missing in context');
        }

        $merchantReference = "pay_{$paymentId}_ord_{$orderId}";

        $returnUrl = (string) config('services.adyen.return_url'); // 例: http://localhost/thanks/buy/adyen?order_id=...

        $payload = [
            'merchantAccount' => $merchantAccount,
            'amount' => [
                'value' => $amount,
                'currency' => strtoupper($currency),
            ],
            'reference' => $merchantReference,
            'returnUrl' => $returnUrl,
            'countryCode' => 'JP',
            'shopperLocale' => 'ja-JP',

            // Drop-in前提。paymentMethodsResponse は不要（sessionで取得できる）
        ];

        $res = Http::withHeaders([
                'X-API-Key' => $apiKey,
                'Content-Type' => 'application/json',
            ])
            ->post(rtrim($baseUrl, '/') . '/v70/sessions', $payload);

        if (!$res->ok()) {
            throw new \RuntimeException('Adyen sessions failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();

        return [
            'provider_payment_id' => (string)($json['id'] ?? ''), // session_id
            'session_id' => (string)($json['id'] ?? ''),
            'session_data' => (string)($json['sessionData'] ?? ''),
            'client_key' => (string) config('services.adyen.client_key'),
            'environment' => $environment === 'live' ? 'live' : 'test',
            'status' => null,
        ];
    }
}