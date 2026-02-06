<?php

namespace App\Modules\Payment\Domain\Repository;

interface PaymentQueryRepository
{
    public function reserve(
        string $provider,
        string $eventId,          // sha256 hex64 (internal idempotency key)
        string $providerEventId,  // evt_... / pspReference
        string $eventType,
        string $payloadHash
    ): bool;

    public function complete(
        string $provider,
        string $eventId,
        string $status,
        ?int $paymentId = null,
        ?int $orderId = null,
        ?string $errorMessage = null,
        ?string $errorCode = null,
    ): void;

    public function findWebhookEventByEventId(string $providerEventId): ?array;

    public function findWebhookEvent(string $provider, string $eventId): ?array;

    public function findWebhookEventByIdempotencyKey(string $eventId): ?array;
}