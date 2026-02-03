<?php

namespace App\Modules\Payment\Domain\Port;

use App\Modules\Payment\Domain\Enum\PaymentMethod;

interface PaymentGatewayPort
{
    public function createIntent(
        PaymentMethod $method,
        int $amount,
        string $currency,
        array $context
    ): array;

    public function createOneClickIntent(
        string $providerCustomerId,
        string $providerPaymentMethodId,
        int $amount,
        string $currency,
        array $context
    ): array;

    /**
     * ✅ Adyen: Checkout Sessions (Drop-in)
     *
     * Return keys:
     * - provider_payment_id (string)   // session_id を入れる（Payment側で保持）
     * - session_id (string)
     * - session_data (string)
     * - client_key (string)
     * - environment ('test'|'live')
     * - status (string|null)
     */
    public function createSession(
        PaymentMethod $method,
        int $amount,
        string $currency,
        array $context
    ): array;
}