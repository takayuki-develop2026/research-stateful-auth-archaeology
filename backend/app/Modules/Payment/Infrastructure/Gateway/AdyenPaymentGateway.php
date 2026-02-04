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

        // Context
        $paymentId = (int) ($context['payment_id'] ?? 0);
        $orderId   = (int) ($context['order_id'] ?? 0);

        if ($paymentId <= 0 || $orderId <= 0) {
            throw new \RuntimeException('payment_id/order_id missing in context');
        }

        // ✅ 最重要：webhook の merchantReference から order_id を取れるようにする
        // -> 文字列として "2" のように入れる
        $merchantReference = (string) $orderId;

        $payload = [
            'merchantAccount' => $merchantAccount,
            'amount' => [
                'value'    => $amount,
                'currency' => strtoupper($currency),
            ],
            'reference'    => $merchantReference,
            'returnUrl'    => $returnUrl,
            'countryCode'  => 'JP',
            'shopperLocale'=> 'ja-JP',

            // Drop-in Sessions 前提
        ];

        $url = rtrim($baseUrl, '/') . '/v70/sessions';

        $res = Http::withHeaders([
                'X-API-Key'     => $apiKey,
                'Content-Type'  => 'application/json',
            ])
            ->post($url, $payload);

        // ✅ 201 Created も成功なので successful() を使う
        if (!$res->successful()) {
            throw new \RuntimeException('Adyen sessions failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();
        if (!is_array($json)) {
            throw new \RuntimeException('Adyen sessions invalid json: ' . $res->body());
        }

        // Adyen Sessions response:
        // - id          => sessionId
        // - sessionData => sessionData
        $sessionId   = $json['id'] ?? null;
        $sessionData = $json['sessionData'] ?? null;

        if (!is_string($sessionId) || $sessionId === '' || !is_string($sessionData) || $sessionData === '') {
            throw new \RuntimeException('Adyen sessions missing id/sessionData: ' . json_encode($json));
        }

        return [
            // provider_payment_id はこの段階では “sessionId” を一旦入れておく（後でpspReferenceで上書き）
            'provider_payment_id' => $sessionId,

            'session_id'   => $sessionId,
            'session_data' => $sessionData,

            // フロントは NEXT_PUBLIC_ADYEN_CLIENT_KEY を使う想定でも返してOK（互換用）
            'client_key'   => (string) config('services.adyen.client_key', ''),
            'environment'  => ($environment === 'live') ? 'live' : 'test',
            'status'       => null,
        ];
    }
}