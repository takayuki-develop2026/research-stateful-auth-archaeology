<?php

namespace App\Modules\Payment\Domain\Service;

use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Domain\Event\DomainPaymentEvent;
use App\Modules\Payment\Domain\Event\DomainPaymentEventType;

final class StripeEventMapper
{
    public function map(HandlePaymentWebhookInput $input): DomainPaymentEvent
    {
        $payload = $input->payload;
        $object  = $payload['data']['object'] ?? [];

        $providerPaymentId = $this->extractPaymentIntentId($input->eventType, $object);

        if (!is_string($providerPaymentId) || $providerPaymentId === '') {
            return DomainPaymentEvent::ignored($input->occurredAt);
        }

        // konbini は payment_intent.* の時だけ意味がある
        $instructions = null;
        if (str_starts_with($input->eventType, 'payment_intent.')) {
            $instructions = $this->extractKonbiniInstructions($object);
        }

        return match ($input->eventType) {

            // ✅ 成功（最重要）: 最終確定は payment_intent.succeeded のみ
            'payment_intent.succeeded' =>
                new DomainPaymentEvent(
                    DomainPaymentEventType::SUCCEEDED,
                    $providerPaymentId,
                    null,
                    $input->occurredAt,
                    $instructions,
                ),

            // ✅ charge.* は売上確定に使わない（fee-onlyルートで別処理する）
            'charge.succeeded',
            'charge.updated',
            'charge.captured',
            'charge.failed' =>
                DomainPaymentEvent::ignored($input->occurredAt),

            'payment_intent.payment_failed' =>
                new DomainPaymentEvent(
                    DomainPaymentEventType::FAILED,
                    $providerPaymentId,
                    $object['last_payment_error']['message'] ?? null,
                    $input->occurredAt,
                    $instructions,
                ),

            'payment_intent.requires_action',
            'payment_intent.created' =>
                new DomainPaymentEvent(
                    DomainPaymentEventType::REQUIRES_ACTION,
                    $providerPaymentId,
                    null,
                    $input->occurredAt,
                    $instructions,
                ),

            // ✅ Refund（成功だけ拾う）
            // Stripe event 的に refund は複数経路があり得るが、最小運用では以下だけでOK
            'charge.refunded' => $this->mapChargeRefunded($providerPaymentId, $object, $input->occurredAt),

            // refund.updated は「状態変化」が来るので、succeeded の時だけ REFUND_SUCCEEDED にする
            'refund.updated' => $this->mapRefundUpdated($providerPaymentId, $object, $input->occurredAt),

            default =>
                DomainPaymentEvent::ignored($input->occurredAt),
        };
    }

    private function mapChargeRefunded(string $providerPaymentId, array $chargeObject, \DateTimeImmutable $occurredAt): DomainPaymentEvent
    {
        $refund = $chargeObject['refunds']['data'][0] ?? null;
        $refundId = is_array($refund) ? ($refund['id'] ?? null) : null;
        $refundAmount = is_array($refund) ? ($refund['amount'] ?? null) : null;

        if (!is_string($refundId) || $refundId === '') {
            return DomainPaymentEvent::ignored($occurredAt);
        }
        if (!is_numeric($refundAmount) || (int)$refundAmount <= 0) {
            return DomainPaymentEvent::ignored($occurredAt);
        }

        return new DomainPaymentEvent(
            DomainPaymentEventType::REFUND_SUCCEEDED,
            $providerPaymentId,
            null,
            $occurredAt,
            [
                'provider' => 'stripe',
                'provider_refund_id' => $refundId,
                'refund_amount' => (int)$refundAmount,
                'currency' => strtoupper($chargeObject['currency'] ?? 'jpy'),
                'reason' => 'stripe_charge.refunded',
            ],
        );
    }

    private function mapRefundUpdated(string $providerPaymentId, array $refundObject, \DateTimeImmutable $occurredAt): DomainPaymentEvent
    {
        // refund.updated の object は refund で、status が来る（成功以外もある）
        $status = $refundObject['status'] ?? null;

        if (!is_string($status) || $status !== 'succeeded') {
            return DomainPaymentEvent::ignored($occurredAt);
        }

        $refundId = $refundObject['id'] ?? null;
        $refundAmount = $refundObject['amount'] ?? null;

        if (!is_string($refundId) || $refundId === '') {
            return DomainPaymentEvent::ignored($occurredAt);
        }
        if (!is_numeric($refundAmount) || (int)$refundAmount <= 0) {
            return DomainPaymentEvent::ignored($occurredAt);
        }

        return new DomainPaymentEvent(
            DomainPaymentEventType::REFUND_SUCCEEDED,
            $providerPaymentId,
            null,
            $occurredAt,
            [
                'provider' => 'stripe',
                'provider_refund_id' => $refundId,
                'refund_amount' => (int)$refundAmount,
                'currency' => strtoupper($refundObject['currency'] ?? 'jpy'),
                'reason' => 'stripe_refund.updated_succeeded',
            ],
        );
    }

    private function extractPaymentIntentId(string $eventType, array $object): ?string
    {
        if (str_starts_with($eventType, 'payment_intent.')) {
            return $object['id'] ?? null;
        }

        if (str_starts_with($eventType, 'charge.')) {
            return $object['payment_intent'] ?? null;
        }

        // ✅ refund.* は object.payment_intent
        if (str_starts_with($eventType, 'refund.')) {
            return $object['payment_intent'] ?? null;
        }

        return null;
    }

    private function extractKonbiniInstructions(array $piObject): ?array
    {
        $details = $piObject['next_action']['konbini_display_details'] ?? null;
        if (!is_array($details)) {
            return null;
        }

        return [
            'type' => 'konbini',
            'expires_at' => $details['expires_at'] ?? null,
            'store' => [
                ($details['store'] ?? '') => [
                    'confirmation_number' => $details['confirmation_number'] ?? null,
                ],
            ],
        ];
    }
}
