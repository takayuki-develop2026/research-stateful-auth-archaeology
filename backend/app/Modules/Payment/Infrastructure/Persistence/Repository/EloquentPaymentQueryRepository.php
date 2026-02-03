<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository;

use App\Modules\Payment\Domain\Repository\PaymentQueryRepository;
use Illuminate\Database\QueryException;
use Illuminate\Support\Facades\DB;

final class EloquentPaymentQueryRepository implements PaymentQueryRepository
{
    public function reserve(
        string $provider,
        string $eventId,
        string $eventType,
        string $payloadHash
    ): bool {
        // ✅ 1) processed_webhook_events にロックを取る（冪等の正：R5）
        try {
            DB::table('processed_webhook_events')->insert([
                'provider'     => $provider,
                'event_id'     => $eventId,
                'event_type'   => $eventType,
                'payload_hash' => $payloadHash,
                'status'       => 'reserved',
                'created_at'   => now(),
                'updated_at'   => now(),
            ]);
        } catch (QueryException $e) {
            $sqlState = $e->errorInfo[0] ?? null;
            $driverCode = $e->errorInfo[1] ?? null;

            if ($sqlState === '23000' || $sqlState === '23505' || $driverCode === 1062) {
                return false; // duplicate = already reserved/processed
            }
            throw $e;
        }

        // ✅ 2) payment_webhook_events は観測ログ（最小：今のUNIQUEを維持しつつinsert）
        // ここがUNIQUEで落ちても冪等ロックは取れているので、例外は握り潰して良い
        try {
            DB::table('payment_webhook_events')->insert([
                'provider'     => $provider,
                'event_id'     => $eventId,
                'event_type'   => $eventType,
                'payload_hash' => $payloadHash,
                'status'       => 'processing',
                'created_at'   => now(),
                'updated_at'   => now(),
            ]);
        } catch (\Throwable) {
            // swallow
        }

        return true;
    }

    public function complete(
        string $provider,
        string $eventId,
        string $status,
        ?int $paymentId = null,
        ?int $orderId = null,
        ?string $errorMessage = null,
    ): void {
        // ✅ processed_webhook_events を確定
        DB::table('processed_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId)
            ->update([
                'status'        => $status,
                'payment_id'    => $paymentId,
                'order_id'      => $orderId,
                'error_message' => $errorMessage,
                'processed_at'  => now(),
                'updated_at'    => now(),
            ]);

        // ✅ payment_webhook_events はログ更新（存在しない場合があるので update のみ）
        DB::table('payment_webhook_events')
            ->where('provider', $provider)
            ->where('event_id', $eventId)
            ->update([
                'status'        => $status,
                'payment_id'    => $paymentId,
                'order_id'      => $orderId,
                'error_message' => $errorMessage,
                'updated_at'    => now(),
            ]);
    }

    public function findWebhookEvent(string $provider, string $eventId): ?array
{
    $event = \DB::table('payment_webhook_events')
        ->where('provider', $provider)
        ->where('event_id', $eventId)
        ->first();

    if (!$event) {
        return null;
    }

    $processed = \DB::table('processed_webhook_events')
        ->where('provider', $provider)
        ->where('event_id', $eventId)
        ->orderByDesc('id')
        ->first();

    return [
        // payment_webhook_events
        'id' => $event->id,
        'provider' => $event->provider,
        'event_id' => $event->event_id,
        'event_type' => $event->event_type,
        'payload_hash' => $event->payload_hash,
        'payload_is_null' => $event->payload === null,
        'status' => $event->status,
        'payment_id' => $event->payment_id,
        'payload' => $event->payload, // JSON（castsしていれば array）
        'signature' => $event->signature,
        'order_id' => $event->order_id,
        'error_message' => $event->error_message,
        'created_at' => (string) $event->created_at,
        'updated_at' => (string) $event->updated_at,

        // processed_webhook_events（存在すれば）
        'processed' => $processed ? [
            'id' => $processed->id,
            'status' => $processed->status,
            'error_code' => $processed->error_code,
            'error_message' => $processed->error_message,
            'processed_at' => (string) $processed->processed_at,
            'created_at' => (string) $processed->created_at,
        ] : null,
    ];
}
}
