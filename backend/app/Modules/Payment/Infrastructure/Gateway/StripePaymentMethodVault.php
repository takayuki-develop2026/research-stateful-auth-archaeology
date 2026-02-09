<?php

namespace App\Modules\Payment\Infrastructure\Gateway;

use App\Modules\Payment\Domain\Service\PaymentMethodVault;
use App\Modules\Payment\Domain\Service\SetupIntentResult;
use App\Modules\Payment\Domain\Service\PaymentMethodCardSnapshot;
use App\Modules\Payment\Domain\Service\SetupIntentSnapshot;
use Stripe\StripeClient;

final class StripePaymentMethodVault implements PaymentMethodVault
{
    private StripeClient $stripe;

    public function __construct()
    {
        $this->stripe = new StripeClient([
            'api_key' => config('services.stripe.secret'),
            'stripe_version' => config('services.stripe.api_version'),
        ]);
    }

    public function createCustomer(int $userId, ?string $email = null, ?string $name = null): string
    {
        $customer = $this->stripe->customers->create([
            'email' => $email,
            'name' => $name,
            'metadata' => [
                'user_id' => (string)$userId,
            ],
        ]);

        return $customer->id;
    }

    public function createSetupIntent(string $providerCustomerId): SetupIntentResult
    {
        $si = $this->stripe->setupIntents->create([
            'customer' => $providerCustomerId,
            'payment_method_types' => ['card'],
            'usage' => 'off_session',
            'metadata' => [
                'purpose' => 'wallet_card_save',
            ],
        ]);

        return new SetupIntentResult(
            setupIntentId: $si->id,
            clientSecret: (string)$si->client_secret,
        );
    }

    public function retrieveSetupIntent(string $setupIntentId): SetupIntentSnapshot
    {
        $si = $this->stripe->setupIntents->retrieve($setupIntentId, []);

        $pm = $si->payment_method ?? null;
        $pmId = null;
        if (is_string($pm)) {
            $pmId = $pm;
        } elseif (is_object($pm) && isset($pm->id)) {
            $pmId = (string)$pm->id;
        }

        $customer = $si->customer ?? null;
        $customerId = null;
        if (is_string($customer)) {
            $customerId = $customer;
        } elseif (is_object($customer) && isset($customer->id)) {
            $customerId = (string)$customer->id;
        }

        return new SetupIntentSnapshot(
            setupIntentId: (string)$si->id,
            status: (string)$si->status,
            providerCustomerId: $customerId,
            providerPaymentMethodId: $pmId,
        );
    }

    public function retrievePaymentMethodCard(string $providerPaymentMethodId): PaymentMethodCardSnapshot
    {
        $pm = $this->stripe->paymentMethods->retrieve($providerPaymentMethodId, []);
        $card = $pm->card ?? null;

        return new PaymentMethodCardSnapshot(
            brand: $card?->brand ?? null,
            last4: $card?->last4 ?? null,
            expMonth: isset($card?->exp_month) ? (int)$card->exp_month : null,
            expYear: isset($card?->exp_year) ? (int)$card->exp_year : null,
        );
    }

    public function detachPaymentMethod(string $providerPaymentMethodId): void
    {
        $this->stripe->paymentMethods->detach($providerPaymentMethodId, []);
    }
}