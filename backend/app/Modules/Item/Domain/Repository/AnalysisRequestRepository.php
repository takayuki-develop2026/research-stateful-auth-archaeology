<?php

declare(strict_types=1);

namespace App\Modules\Item\Domain\Repository;

use App\Modules\Item\Domain\ValueObject\AnalysisRequestRecord;

interface AnalysisRequestRepository
{
    public function create(array $attributes): int;

    public function findOrFail(int $requestId): AnalysisRequestRecord;

    public function markDone(int $requestId): void;

    public function markFailed(
        int $requestId,
        ?string $errorCode = null,
        ?string $errorMessage = null,
    ): void;

    /**
     * Atlas 管理画面用
     * @return array<int,\stdClass>
     */
    public function listByShopCode(string $shopCode): array;




/**
     * @return array<int, \stdClass>
     */
    public function listAllShops(): array;
}