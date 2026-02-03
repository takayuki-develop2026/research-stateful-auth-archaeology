<?php

namespace App\Modules\Payment\Infrastructure\Gateway;

use App\Modules\Payment\Domain\Enum\PaymentMethod;
use App\Modules\Payment\Domain\Port\PaymentGatewayPort;
use Stripe\StripeClient;

final class StripePaymentGateway implements PaymentGatewayPort
{
    private StripeClient $stripe;

    public function __construct()
    {
        $this->stripe = new StripeClient([
            'api_key' => config('services.stripe.secret'),
            'stripe_version' => config('services.stripe.api_version'),
        ]);
    }

    public function createIntent(
        PaymentMethod $method,
        int $amount,
        string $currency,
        array $context
    ): array {
        $metadata = [
            'order_id'   => (string) ($context['order_id'] ?? ''),
            'payment_id' => (string) ($context['payment_id'] ?? ''), // ✅ CARDも必ず入れる（R3）
            'user_id'    => (string) ($context['user_id'] ?? ''),
            'shop_id'    => (string) ($context['shop_id'] ?? ''),
        ];

        // 空文字が入るのが気になるならここで unsetしても良いが、救済目的なら残してOK
        // unset empty values
        foreach ($metadata as $k => $v) {
            if ($v === '') unset($metadata[$k]);
        }

        if ($method === PaymentMethod::CARD) {
            $pi = $this->stripe->paymentIntents->create([
                'amount'   => $amount,
                'currency' => strtolower($currency),
                'payment_method_types' => ['card'],
                'metadata' => $metadata,
            ]);

            return [
                'provider_payment_id' => $pi->id,
                'client_secret'       => $pi->client_secret,
                'requires_action'     => $pi->status === 'requires_action',
                'status'              => $pi->status,
            ];
        }

        if ($method === PaymentMethod::KONBINI) {
            $payerName  = $context['payer_name'] ?? '購入者';
            $payerEmail = $context['payer_email'] ?? 'no-reply@example.com';

            $pi = $this->stripe->paymentIntents->create([
                'amount'   => $amount,
                'currency' => 'jpy',
                'confirm'  => true,

                'payment_method_types' => ['konbini'],
                'payment_method_data' => [
                    'type' => 'konbini',
                    'billing_details' => [
                        'name'  => $payerName,
                        'email' => $payerEmail,
                    ],
                ],

                'metadata' => $metadata,
            ]);

            $details = $pi->next_action->konbini_display_details ?? null;

            return [
                'provider_payment_id' => $pi->id,
                'client_secret'       => $pi->client_secret,
                'requires_action'     => true,
                'status'              => $pi->status,
                'instructions' => [
                    'type'       => 'konbini',
                    'expires_at' => $details?->expires_at,
                    'reference'  => $details?->confirmation_number,
                    'store'      => $details?->stores,
                ],
            ];
        }

        throw new \InvalidArgumentException('Unsupported payment method');
    }

    public function createOneClickIntent(
    string $providerCustomerId,
    string $providerPaymentMethodId,
    int $amount,
    string $currency,
    array $context
): array {
    // ✅ metadata（R3救済で使う payment_id も入れる）
    $metadata = [
        'order_id'   => (string)($context['order_id'] ?? ''),
        'payment_id' => (string)($context['payment_id'] ?? ''),
        'user_id'    => (string)($context['user_id'] ?? ''),
        'shop_id'    => (string)($context['shop_id'] ?? ''),
    ];

    foreach ($metadata as $k => $v) {
        if ($v === '') unset($metadata[$k]);
    }

    $pi = $this->stripe->paymentIntents->create([
        'amount' => $amount,
        'currency' => strtolower($currency),

        'customer' => $providerCustomerId,
        'payment_method' => $providerPaymentMethodId,
        'payment_method_types' => ['card'],

        // ✅ OneClick（即confirm）: 3DSが必要なら requires_action + client_secret が返る
        'confirm' => true,

        'metadata' => $metadata,
    ]);

    return [
        'provider_payment_id' => $pi->id,
        'client_secret' => $pi->client_secret,
        'requires_action' => ($pi->status === 'requires_action'),
        'status' => $pi->status,
    ];
}

public function createSession(
    PaymentMethod $method,
    int $amount,
    string $currency,
    array $context
): array {
    throw new \LogicException('Stripe does not support createSession(); use createIntent().');
}

}