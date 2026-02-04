<?php

declare(strict_types=1);

namespace App\Modules\Payment\Domain\Service;

use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Domain\Event\DomainPaymentEventType;

final class AdyenEventMapper
{
    public function map(HandlePaymentWebhookInput $input): object
    {
        $nri = $input->payload; // NRI array

        $eventCode = (string)($nri['eventCode'] ?? '');
        $success = (string)($nri['success'] ?? 'false');
        $pspRef = (string)($nri['pspReference'] ?? '');
        $merchantRef = (string)($nri['merchantReference'] ?? '');

        $type = DomainPaymentEventType::IGNORED;

        // For MVP: AUTHORISATION only
        if ($eventCode === 'AUTHORISATION') {
            $type = ($success === 'true')
                ? DomainPaymentEventType::SUCCEEDED
                : DomainPaymentEventType::FAILED;
        }

        // DomainEvent “shape” to match your existing UseCase usage
        return (object) [
            'type' => $type,
            'providerPaymentId' => $pspRef,            // Adyen PSP reference
            'occurredAt' => $input->occurredAt,
            'reason' => $nri['reason'] ?? null,
            'instructions' => [
                'merchantReference' => $merchantRef,
                'success' => $success,
                'eventCode' => $eventCode,
                'pspReference' => $pspRef,
            ],
        ];
    }
}