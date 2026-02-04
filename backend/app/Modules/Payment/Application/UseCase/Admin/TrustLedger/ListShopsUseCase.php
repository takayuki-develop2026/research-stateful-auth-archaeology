<?php

namespace App\Modules\Payment\Application\UseCase\Admin\TrustLedger;

use App\Modules\Payment\Domain\Shop\Repository\AdminShopQueryRepository;

final class ListShopsUseCase
{
    public function __construct(private AdminShopQueryRepository $shops) {}

    public function handle(?string $q, ?string $status, ?string $provider, int $limit, int $page): array
    {
        return $this->shops->search($q, $status, $provider, $limit, $page);
    }
}