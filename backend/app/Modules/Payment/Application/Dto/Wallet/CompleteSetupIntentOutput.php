<?php

namespace App\Modules\Payment\Application\Dto\Wallet;

final class CompleteSetupIntentOutput
{
    public function __construct(
        public bool $ok,
        public int $walletId,
        public string $provider,
        public string $providerPaymentMethodId,
        public bool $isDefault,
    ) {
    }

    public function toArray(): array
    {
        return [
            'ok' => $this->ok,
            'wallet_id' => $this->walletId,
            'provider' => $this->provider,
            'provider_payment_method_id' => $this->providerPaymentMethodId,
            'is_default' => $this->isDefault,
        ];
    }
}