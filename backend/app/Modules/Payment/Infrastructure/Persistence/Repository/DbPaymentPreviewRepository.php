<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository;

use App\Modules\Payment\Domain\Repository\PaymentPreviewRepository;
use Illuminate\Support\Facades\DB;

final class DbPaymentPreviewRepository implements PaymentPreviewRepository
{
    public function create(array $data): void
    {
        DB::table('payment_previews')->insert([
            ...$data,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    public function findActiveByKey(string $previewKey): ?object
    {
        return DB::table('payment_previews')
            ->where('preview_key', $previewKey)
            ->where('status', 'created')
            ->where(function ($q) {
                $q->whereNull('expires_at')->orWhere('expires_at', '>', now());
            })
            ->first();
    }

    public function findByKey(string $previewKey): ?object
    {
        return DB::table('payment_previews')
            ->where('preview_key', $previewKey)
            ->first();
    }

    public function findCommittedByKey(string $previewKey): ?object
    {
        return DB::table('payment_previews')
            ->where('preview_key', $previewKey)
            ->where('status', 'committed')
            ->first();
    }

    public function markCommitted(string $previewKey, int $orderId, int $paymentId): void
    {
        DB::table('payment_previews')
            ->where('preview_key', $previewKey)
            ->update([
                'status' => 'committed',
                'order_id' => $orderId,
                'payment_id' => $paymentId,
                'updated_at' => now(),
            ]);
    }

    public function markExpiredByKey(string $previewKey): void
    {
        DB::table('payment_previews')
            ->where('preview_key', $previewKey)
            ->update([
                'status' => 'expired',
                'updated_at' => now(),
            ]);
    }
}