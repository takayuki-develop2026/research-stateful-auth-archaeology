<?php

namespace App\Modules\Payment\Application\Dto;

final class StartPaymentOutput
{
    public function __construct(
        public readonly string $provider, // 'stripe' | 'adyen'
        public readonly int $paymentId,
        public readonly string $status,
        public readonly ?string $providerPaymentId,

        // Stripe
        public readonly ?string $clientSecret,

        // Adyen (Sessions + Drop-in)
        public readonly ?string $sessionId,
        public readonly ?string $sessionData,
        public readonly ?string $clientKey,
        public readonly ?string $environment, // 'test' | 'live'

        public readonly ?array $instructions,
    ) {
    }

    public function toArray(): array
    {
        return [
            'provider' => $this->provider,
            'payment_id' => $this->paymentId,
            'status' => $this->status,
            'provider_payment_id' => $this->providerPaymentId,

            // Stripe
            'client_secret' => $this->clientSecret,

            // Adyen
            'session_id' => $this->sessionId,
            'session_data' => $this->sessionData,
            'client_key' => $this->clientKey,
            'environment' => $this->environment,

            'instructions' => $this->instructions,
        ];
    }
}