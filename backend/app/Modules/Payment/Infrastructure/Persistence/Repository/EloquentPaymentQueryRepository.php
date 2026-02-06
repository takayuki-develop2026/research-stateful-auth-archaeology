<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository;

use App\Modules\Payment\Domain\Repository\PaymentQueryRepository;
use Illuminate\Database\QueryException;
use Illuminate\Support\Facades\DB;

final class EloquentPaymentQueryRepository implements PaymentQueryRepository
{
    /**
     * 冪等キー（event_id）は sha256(hex64) を受け取る前提
     * provider_event_id は Stripe evt_... / Adyen pspReference 等
     */
    public function reserve(
        string $provider,
        string $eventId,          // sha256 fixed key (hex64)
        string $providerEventId,  // evt_... / pspReference
        string $eventType,
        string $payloadHash
    ): bool {
        // 1) processed_webhook_events に冪等ロック（SoT）
        try {
            DB::table('processed_webhook_events')->insert([
                'provider'          => $provider,
                'event_id'          => $eventId,          // sha256
                'provider_event_id' => $providerEventId,  // evt_...
                'event_type'        => $eventType,
                'payload_hash'      => $payloadHash,
                'status'            => 'reserved',
                'created_at'        => now(),
                'updated_at'        => now(),
            ]);
        } catch (QueryException $e) {
            $sqlState = $e->errorInfo[0] ?? null;
            $driverCode = $e->errorInfo[1] ?? null;

            // duplicate key
            if ($sqlState === '23000' || $sqlState === '23505' || $driverCode === 1062) {
                return false;
            }
            throw $e;
        }

        // 2) payment_webhook_events は観測ログ（失敗しても冪等ロックは成立してるので握りつぶしOK）
        try {
            DB::table('payment_webhook_events')->insert([
                'provider'          => $provider,
                'event_id'          => $eventId,         // sha256
                'provider_event_id' => $providerEventId, // evt_...
                'event_type'        => $eventType,
                'payload_hash'      => $payloadHash,
                'status'            => 'processing',
                'created_at'        => now(),
                'updated_at'        => now(),
            ]);
        } catch (\Throwable) {
            // swallow
        }

        return true;
    }

    public function complete(
        string $provider,
        string $eventId,         // sha256
        string $status,
        ?int $paymentId = null,
        ?int $orderId = null,
        ?string $errorMessage = null,
        ?string $errorCode = null,
    ): void {
        DB::table('processed_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId) // ✅ sha256で確定更新
            ->update([
                'status'        => $status,
                'payment_id'    => $paymentId,
                'order_id'      => $orderId,
                'error_code'    => $errorCode,
                'error_message' => $errorMessage,
                'processed_at'  => now(),
                'updated_at'    => now(),
            ]);

        DB::table('payment_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId) // ✅ sha256で追随更新
            ->update([
                'status'        => $status,
                'payment_id'    => $paymentId,
                'order_id'      => $orderId,
                'error_message' => $errorMessage,
                'updated_at'    => now(),
            ]);
    }

    /**
     * Admin 詳細：主語は provider_event_id（evt_...）
     * payload は payment_webhook_events から取れる（保存していれば）
     */
    public function findWebhookEventByEventId(string $providerEventId): ?array
    {
        // まず観測ログ（payload など）
        $event = DB::table('payment_webhook_events')
            ->where('provider_event_id', $providerEventId)
            ->orderByDesc('id')
            ->first();

        // 冪等SoT
        $processed = DB::table('processed_webhook_events')
            ->where('provider_event_id', $providerEventId)
            ->orderByDesc('id')
            ->first();

        // どっちも無ければ NotFound
        if (!$event && !$processed) {
            return null;
        }

        // event_id(sha256) は processed を優先（無い場合は event から）
        $eventId = $processed->event_id ?? ($event->event_id ?? null);
        $provider = $processed->provider ?? ($event->provider ?? null);

        return [
            'provider' => $provider ? (string)$provider : null,

            // sha256 固定キー
            'event_id' => $eventId ? (string)$eventId : null,

            // PSP ID
            'provider_event_id' => $providerEventId,

            // 型・ステータス（観測ログを優先、なければ processed）
            'event_type' => $event->event_type ?? ($processed->event_type ?? null),
            'status' => $event->status ?? ($processed->status ?? null),

            // payload系（保存しているなら event から）
            'payload_hash' => $event->payload_hash ?? ($processed->payload_hash ?? null),
            'payload' => $event->payload ?? null,
            'payload_is_null' => ($event ? $event->payload === null : true),

            // 参照キー
            'payment_id' => $event->payment_id ?? ($processed->payment_id ?? null),
            'order_id' => $event->order_id ?? ($processed->order_id ?? null),

            // エラー
            'error_code' => $processed->error_code ?? null,
            'error_message' => $event->error_message ?? ($processed->error_message ?? null),

            // timestamps
            'created_at' => (string)($event->created_at ?? $processed->created_at ?? now()),
            'updated_at' => (string)($event->updated_at ?? $processed->updated_at ?? now()),

            // processed 詳細
            'processed' => $processed ? [
                'id' => (int)$processed->id,
                'status' => (string)$processed->status,
                'processed_at' => $processed->processed_at ? (string)$processed->processed_at : null,
                'created_at' => (string)$processed->created_at,
                'updated_at' => (string)$processed->updated_at,
            ] : null,
        ];
    }

    /**
     * 旧：provider + event_id で引きたい内部用途があるなら残してOK
     * ただし event_id は sha256固定キーの意味に限定する
     */
    public function findWebhookEvent(string $provider, string $eventId): ?array
    {
        $event = DB::table('payment_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId) // sha256
            ->orderByDesc('id')
            ->first();

        if (!$event) {
            return null;
        }

        $processed = DB::table('processed_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId) // sha256
            ->orderByDesc('id')
            ->first();

        return [
            'provider' => (string)$event->provider,
            'event_id' => (string)$event->event_id,
            'provider_event_id' => (string)($event->provider_event_id ?? ''),
            'event_type' => (string)$event->event_type,
            'payload_hash' => (string)$event->payload_hash,
            'payload_is_null' => $event->payload === null,
            'status' => (string)$event->status,
            'payment_id' => $event->payment_id,
            'order_id' => $event->order_id,
            'payload' => $event->payload,
            'error_message' => $event->error_message,
            'created_at' => (string)$event->created_at,
            'updated_at' => (string)$event->updated_at,
            'processed' => $processed ? [
                'id' => (int)$processed->id,
                'status' => (string)$processed->status,
                'error_code' => $processed->error_code,
                'error_message' => $processed->error_message,
                'processed_at' => $processed->processed_at ? (string)$processed->processed_at : null,
                'created_at' => (string)$processed->created_at,
            ] : null,
        ];
    }

    public function findWebhookEventByIdempotencyKey(string $eventId): ?array
{
    // 観測ログ（payload）
    $event = DB::table('payment_webhook_events')
        ->where('event_id', $eventId) // sha256
        ->orderByDesc('id')
        ->first();

    // 冪等SoT
    $processed = DB::table('processed_webhook_events')
        ->where('event_id', $eventId) // sha256
        ->orderByDesc('id')
        ->first();

    if (!$event && !$processed) {
        return null;
    }

    $providerEventId = $processed->provider_event_id ?? ($event->provider_event_id ?? null);

    return [
        'provider' => (string)($processed->provider ?? $event->provider ?? ''),
        'event_id' => (string)$eventId, // sha256
        'provider_event_id' => $providerEventId ? (string)$providerEventId : null,
        'event_type' => $event->event_type ?? ($processed->event_type ?? null),
        'status' => $event->status ?? ($processed->status ?? null),
        'payload_hash' => $event->payload_hash ?? ($processed->payload_hash ?? null),
        'payload' => $event->payload ?? null,
        'payload_is_null' => ($event ? $event->payload === null : true),
        'payment_id' => $event->payment_id ?? ($processed->payment_id ?? null),
        'order_id' => $event->order_id ?? ($processed->order_id ?? null),
        'error_code' => $processed->error_code ?? null,
        'error_message' => $event->error_message ?? ($processed->error_message ?? null),
        'created_at' => (string)($event->created_at ?? $processed->created_at ?? now()),
        'updated_at' => (string)($event->updated_at ?? $processed->updated_at ?? now()),
        'processed' => $processed ? [
            'id' => (int)$processed->id,
            'status' => (string)$processed->status,
            'processed_at' => $processed->processed_at ? (string)$processed->processed_at : null,
            'created_at' => (string)$processed->created_at,
            'updated_at' => (string)$processed->updated_at,
        ] : null,
    ];
}
}