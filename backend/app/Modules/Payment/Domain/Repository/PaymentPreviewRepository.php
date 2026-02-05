<?php

namespace App\Modules\Payment\Domain\Repository;

interface PaymentPreviewRepository
{
    public function create(array $data): void;

    /**
     * created + not expired のみ
     */
    public function findActiveByKey(string $previewKey): ?object;

    /**
     * status 問わず取得（監査・調査用にも使える）
     */
    public function findByKey(string $previewKey): ?object;

    /**
     * committed のみ取得（Webhook で使う正統ルート）
     */
    public function findCommittedByKey(string $previewKey): ?object;

    public function markCommitted(string $previewKey, int $orderId, int $paymentId): void;

    public function markExpiredByKey(string $previewKey): void;
}