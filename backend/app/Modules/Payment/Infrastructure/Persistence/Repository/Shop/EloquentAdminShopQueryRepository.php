<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository\Shop;

use App\Modules\Payment\Domain\Shop\Repository\AdminShopQueryRepository;
use Illuminate\Support\Facades\DB;

final class EloquentAdminShopQueryRepository implements AdminShopQueryRepository
{
    public function search(?string $q, ?string $status, ?string $provider, int $limit, int $page): array
    {
        $limit  = max(1, min(200, $limit));
        $page   = max(1, $page);
        $offset = ($page - 1) * $limit;

        // ✅ owner を「最新付与の owner」に固定
        $latestOwnerRoleUserIdSub = DB::table('role_user as ru')
            ->join('roles as r', 'r.id', '=', 'ru.role_id')
            ->whereNotNull('ru.shop_id')
            ->where('r.slug', '=', 'owner')
            ->select([
                'ru.shop_id',
                DB::raw('MAX(ru.id) as max_ru_id'),
            ])
            ->groupBy('ru.shop_id');

        $ownerSub = DB::table('role_user as ru2')
            ->joinSub($latestOwnerRoleUserIdSub, 'x', function ($join) {
                $join->on('x.max_ru_id', '=', 'ru2.id');
            })
            ->join('users as ou', 'ou.id', '=', 'ru2.user_id')
            ->select([
                'ru2.shop_id',
                'ou.name as owner_name',
            ]);

        $qb = DB::table('shops')
            ->leftJoinSub($ownerSub, 'owners', function ($join) {
                $join->on('owners.shop_id', '=', 'shops.id');
            })
            ->select([
                'shops.id',
                'shops.shop_code',
                'shops.name',
                'shops.status',
                'shops.type',
                'shops.updated_at',
                DB::raw("COALESCE(owners.owner_name, '-') as owner_name"),
                DB::raw("COALESCE(shops.payment_provider, 'stripe') as payment_provider"),
            ]);

        if ($q) {
            $like = '%' . $q . '%';
            $qb->where(function ($w) use ($like) {
                $w->where('shops.name', 'like', $like)
                  ->orWhere('shops.shop_code', 'like', $like);
            });
        }

        if ($status) {
            $qb->where('shops.status', $status);
        }

        if ($provider) {
            $qb->whereRaw("COALESCE(shops.payment_provider, 'stripe') = ?", [$provider]);
        }

        $total = (clone $qb)->count();

        $rows = $qb->orderBy('shops.id', 'desc')
            ->limit($limit)
            ->offset($offset)
            ->get();

        $items = [];
        foreach ($rows as $r) {
            $items[] = [
                'id'               => (int)$r->id,
                'shop_code'         => $r->shop_code,
                'name'              => $r->name,
                'owner_name'        => $r->owner_name,
                'status'            => $r->status,
                'type'              => $r->type,
                'payment_provider'  => $r->payment_provider,
                'updated_at'        => $r->updated_at,
            ];
        }

        $pages = (int) ceil($total / $limit);

        return [
            'items' => $items,
            'total' => (int) $total,
            'page'  => $page,
            'pages' => max(1, $pages),
            'limit' => $limit,
        ];
    }
}