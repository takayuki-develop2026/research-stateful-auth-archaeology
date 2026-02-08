<?php

namespace App\Modules\Shop\Domain\Repository;

interface ShopRoleQueryRepository
{
    /**
     * @return string[] role slugs for the (userId, shopId)
     */
    public function getRoleSlugsForUserInShop(int $userId, int $shopId): array;

    /**
     * @return bool whether any role exists for (userId, shopId)
     */
    public function existsRoleForUserInShop(int $userId, int $shopId): bool;

    /**
    * @return array<int,array{shop_id:int,shop_code:string,role:string}>
    */
    public function findByUserId(int $userId): array;
}
