<?php

namespace App\Modules\Payment\Domain\Service;

final class SetupIntentSnapshot
{
    public function __construct(
        public readonly string $setupIntentId,
        public readonly string $status, // requires_payment_method / requires_action / processing / succeeded / canceled
        public readonly ?string $providerCustomerId,
        public readonly ?string $providerPaymentMethodId, // pm_xxx
    ) {
    }
}