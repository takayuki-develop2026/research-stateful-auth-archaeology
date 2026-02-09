<?php

namespace App\Modules\Payment\Domain\Service;

final class SetupIntentResult
{
    public function __construct(
        public readonly string $setupIntentId,
        public readonly string $clientSecret,
    ) {
    }
}

final class PaymentMethodCardSnapshot
{
    public function __construct(
        public readonly ?string $brand,
        public readonly ?string $last4,
        public readonly ?int $expMonth,
        public readonly ?int $expYear,
    ) {
    }
}

interface PaymentMethodVault
{
    public function createCustomer(int $userId, ?string $email = null, ?string $name = null): string;

    public function createSetupIntent(string $providerCustomerId): SetupIntentResult;

    // ✅ 追加（complete API の核）
    public function retrieveSetupIntent(string $setupIntentId): SetupIntentSnapshot;

    public function retrievePaymentMethodCard(string $providerPaymentMethodId): PaymentMethodCardSnapshot;

    public function detachPaymentMethod(string $providerPaymentMethodId): void;
}