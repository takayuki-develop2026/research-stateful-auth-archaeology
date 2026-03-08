<?php

declare(strict_types=1);

namespace App\Modules\Item\Application\Query;

use App\Modules\Item\Domain\Repository\AnalysisRequestRepository;

final class AtlasRequestListQuery
{
    public function __construct(
        private AnalysisRequestRepository $repository
    ) {}

    /**
     * @return array<int, \stdClass>
     */
    public function listByShopCode(string $shopCode): array
    {
        return $this->repository->listByShopCode($shopCode);
    }

    /**
     * @return array<int, \stdClass>
     */
    public function listAllShops(): array
    {
        return $this->repository->listAllShops();
    }
}