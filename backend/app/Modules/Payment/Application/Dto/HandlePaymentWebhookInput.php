<?php

namespace App\Modules\Payment\Application\Dto;

final class HandlePaymentWebhookInput
{
    public function __construct(
        public string $provider,
        public string $eventId,          // sha256 hex64
        public string $providerEventId,  // evt_... / pspReference
        public string $eventType,
        public array  $payload,
        public string $payloadHash,
        public \DateTimeImmutable $occurredAt,
    ) {}
}