<?php

namespace App\Modules\Payment\Application\UseCase\Admin\TrustLedger;

use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Application\UseCase\HandlePaymentWebhookUseCase;
use DateTimeImmutable;
use Illuminate\Support\Facades\DB;

final class ReplayWebhookEventUseCase
{
    public function __construct(
        private HandlePaymentWebhookUseCase $handler,
    ) {}

    /**
     * @param string $providerEventId Stripe: evt_... / Adyen: pspReference...
     */
    public function handle(string $providerEventId): array
    {
        return DB::transaction(function () use ($providerEventId) {

            // 1) 保存済み受信イベント取得（payload必須）
            $event = DB::table('payment_webhook_events')
                ->where('provider_event_id', $providerEventId) // ✅ 主語
                ->orWhere('event_id', $providerEventId)        // 保険（過去の揺れ救済）
                ->orderByDesc('id')
                ->first();

            if (!$event) {
                return response()->json([
                    'error_type' => 'NotFound',
                    'message' => 'Webhook event not found.',
                    'provider_event_id' => $providerEventId,
                ], 404)->throwResponse();
            }

            if (!$event->payload) {
                return response()->json([
                    'error_type' => 'Conflict',
                    'message' => 'Payload not stored. Cannot replay.',
                    'provider_event_id' => $providerEventId,
                ], 409)->throwResponse();
            }

            // 2) raw JSON 文字列として正規化
            $raw = is_string($event->payload)
                ? $event->payload
                : json_encode($event->payload, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);

            if (!is_string($raw) || $raw === '') {
                return response()->json([
                    'error_type' => 'Conflict',
                    'message' => 'Stored payload is empty.',
                    'provider_event_id' => $providerEventId,
                ], 409)->throwResponse();
            }

            // 3) payload_hash 整合性チェック（受信時ルールと一致させる）
            $computedHash = hash('sha256', $raw);
            if ((string)$computedHash !== (string)$event->payload_hash) {
                return response()->json([
                    'error_type' => 'Conflict',
                    'message' => 'Payload hash mismatch. Refuse replay.',
                    'provider_event_id' => $providerEventId,
                ], 409)->throwResponse();
            }

            $payloadArr = json_decode($raw, true);
            if (!is_array($payloadArr)) {
                return response()->json([
                    'error_type' => 'Conflict',
                    'message' => 'Stored payload is not valid JSON.',
                    'provider_event_id' => $providerEventId,
                ], 409)->throwResponse();
            }

            // 4) occurredAt（Stripe: created unix seconds）
            $createdUnix = $payloadArr['created'] ?? null;
            if (is_int($createdUnix) || (is_string($createdUnix) && ctype_digit($createdUnix))) {
                $occurredAt = (new DateTimeImmutable())->setTimestamp((int)$createdUnix);
            } else {
                $occurredAt = $event->created_at
                    ? new DateTimeImmutable((string)$event->created_at)
                    : new DateTimeImmutable();
            }

            // 5) 冪等テーブルを replay 用に “見かけ上未処理” に戻す
            // ✅ ここも主語は provider_event_id
            DB::table('processed_webhook_events')
                ->where('provider', (string)$event->provider)
                ->where('provider_event_id', $providerEventId)
                ->update([
                    'status' => 'reserved',
                    'error_code' => null,
                    'error_message' => null,
                    'processed_at' => null,
                    'updated_at' => now(),
                ]);

            // 6) HandlePaymentWebhookInput
            $input = new HandlePaymentWebhookInput(
                provider: (string)$event->provider,
                eventId: (string)($payloadArr['id'] ?? $providerEventId), // ✅ evt_...
                eventType: (string)($payloadArr['type'] ?? $event->event_type),
                payload: $payloadArr,
                payloadHash: (string)$event->payload_hash,
                occurredAt: $occurredAt,
            );

            // 7) 再処理
            $this->handler->handle($input);

            return [
                'status' => 'replayed',
                'provider_event_id' => $providerEventId,
                'occurred_at' => $occurredAt->format(DATE_ATOM),
            ];
        });
    }
}