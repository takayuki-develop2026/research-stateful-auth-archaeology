<?php

namespace App\Modules\Shop\Infrastructure\Persistence;

use App\Modules\Shop\Domain\Repository\ShopRoleQueryRepository;
use Illuminate\Support\Facades\DB;

final class EloquentShopRoleQueryRepository implements ShopRoleQueryRepository
{
    public function getRoleSlugsForUserInShop(int $userId, int $shopId): array
    {
        // role_user(shop_id nullable) の方針に合わせ：
        // - shop_id = 対象shop のロール
        // - shop_id is null のグローバルロール（developer 等）も取得可能にする
        $rows = DB::table('role_user')
            ->join('roles', 'role_user.role_id', '=', 'roles.id')
            ->where('role_user.user_id', $userId)
            ->where(function ($q) use ($shopId) {
                $q->where('role_user.shop_id', $shopId)
                ->orWhereNull('role_user.shop_id');
            })
            ->select('roles.slug')
            ->get();

        return $rows->pluck('slug')->unique()->values()->all();
    }

    public function existsRoleForUserInShop(int $userId, int $shopId): bool
    {
        return DB::table('role_user')
            ->where('user_id', $userId)
            ->where('shop_id', $shopId)
            ->exists();
    }

    public function findByUserId(int $userId): array
{
    return DB::table('role_user')
        ->join('roles', 'roles.id', '=', 'role_user.role_id')
        ->join('shops', 'shops.id', '=', 'role_user.shop_id')
        ->where('role_user.user_id', $userId)
        ->whereNotNull('role_user.shop_id') // ✅ 重要
        ->select([
            'role_user.shop_id',
            'shops.shop_code',
            'roles.slug as role',
        ])
        ->get()
        ->map(fn ($r) => [
            'shop_id'   => (int) $r->shop_id,
            'shop_code' => (string) $r->shop_code,
            'role'      => (string) $r->role,
        ])
        ->all();
}

}
