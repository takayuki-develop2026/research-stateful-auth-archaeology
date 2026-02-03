<?php

namespace App\Modules\Payment\Domain\Repository;

interface PaymentQueryRepository
{
    /**
     * Reserve webhook event (idempotency lock)
     * @return bool true if reserved, false if already processed
     */
    public function reserve(
        string $provider,
        string $eventId,
        string $eventType,
        string $payloadHash
    ): bool;

    /**
     * Mark final processing result
     */
    public function complete(
        string $provider,
        string $eventId,
        string $status,
        ?int $paymentId = null,
        ?int $orderId = null,
        ?string $errorMessage = null,
    ): void;

    public function findWebhookEvent(string $provider, string $eventId): ?array;
}
