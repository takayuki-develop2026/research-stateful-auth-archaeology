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

        $apiKey          = (string) config('services.adyen.api_key');
        $merchantAccount = (string) config('services.adyen.merchant_account');
        $environment     = (string) config('services.adyen.environment', 'test'); // test|live
        $baseUrl         = (string) config('services.adyen.checkout_base_url');   // https://checkout-test.adyen.com
        $returnUrl       = (string) config('services.adyen.return_url');         // http://localhost/thanks/buy/adyen

        // ✅ 最重要：merchantReference は preview_key（uuid）を入れる
        $previewKey = (string)($context['reference'] ?? $context['preview_key'] ?? '');
        if ($previewKey === '') {
            throw new \RuntimeException('reference/preview_key missing in context');
        }

        $payload = [
            'merchantAccount' => $merchantAccount,
            'amount' => [
                'value'    => $amount,
                'currency' => strtoupper($currency),
            ],
            'reference'    => $previewKey, // ✅ merchantReference として通知される
            'returnUrl'    => $returnUrl,
            'countryCode'  => 'JP',
            'shopperLocale'=> 'ja-JP',
        ];

        $url = rtrim($baseUrl, '/') . '/v70/sessions';

        $res = Http::withHeaders([
                'X-API-Key'     => $apiKey,
                'Content-Type'  => 'application/json',
            ])
            ->post($url, $payload);

        if (!$res->successful()) {
            throw new \RuntimeException('Adyen sessions failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();
        if (!is_array($json)) {
            throw new \RuntimeException('Adyen sessions invalid json: ' . $res->body());
        }

        $sessionId   = $json['id'] ?? null;
        $sessionData = $json['sessionData'] ?? null;

        if (!is_string($sessionId) || $sessionId === '' || !is_string($sessionData) || $sessionData === '') {
            throw new \RuntimeException('Adyen sessions missing id/sessionData: ' . json_encode($json));
        }

        return [
            // provider_payment_id は “sessionId” を仮置き（webhook で pspReference に上書き）
            'provider_payment_id' => $sessionId,
            'session_id'   => $sessionId,
            'session_data' => $sessionData,
            'client_key'   => (string) config('services.adyen.client_key', ''),
            'environment'  => ($environment === 'live') ? 'live' : 'test',
            'status'       => null,
        ];
    }
}