<?php

namespace App\Modules\Payment\Domain\Shop\Repository;

interface AdminShopQueryRepository
{
    /**
     * @return array{
     *   items: array<int, array{
     *     id:int,
     *     shop_code:string,
     *     name:string,
     *     owner_name:string,
     *     status:string,
     *     type:string,
     *     payment_provider:string,
     *     updated_at:mixed
     *   }>,
     *   total:int,
     *   page:int,
     *   pages:int,
     *   limit:int
     * }
     */
    public function search(?string $q, ?string $status, ?string $provider, int $limit, int $page): array;
}