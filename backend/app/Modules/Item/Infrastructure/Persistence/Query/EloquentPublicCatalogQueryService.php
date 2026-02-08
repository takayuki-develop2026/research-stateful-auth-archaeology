<?php

namespace App\Modules\Item\Infrastructure\Persistence\Query;

use App\Models\Item;
use App\Modules\Item\Application\Query\PublicCatalogQueryService;
use Illuminate\Contracts\Pagination\LengthAwarePaginator;
use Illuminate\Support\Facades\DB;

final class EloquentPublicCatalogQueryService implements PublicCatalogQueryService
{
    public function paginate(
        int $limit,
        int $page,
        ?string $keyword,
        array $excludeShopIds = []
    ): LengthAwarePaginator {

        $query = Item::query()
            ->whereNotNull('published_at')
            ->select([
                'id',
                'shop_id',
                'created_by_user_id',
                'item_origin',
                'name',
                'price',
                'brand',
                'condition',
                'item_image',
                'remain',
                'published_at',
            ])
            // ✅ 出品者が「何かしら shop_id を持つロール」を持ってるか
            ->selectRaw("
                EXISTS(
                    SELECT 1
                    FROM role_user ru
                    WHERE ru.user_id = items.created_by_user_id
                      AND ru.shop_id IS NOT NULL
                ) as creator_has_shop_role
            ");

        if ($keyword !== null && $keyword !== '') {
            $query->where('name', 'LIKE', '%' . $keyword . '%');
        }

        if (!empty($excludeShopIds)) {
            $query->where(function ($q) use ($excludeShopIds) {
                $q->whereNull('shop_id')
                  ->orWhereNotIn('shop_id', $excludeShopIds);
            });
        }

        $paginator = $query
            ->orderByDesc('published_at')
            ->paginate(
                perPage: $limit,
                columns: ['*'],     // ✅ 重要：select を上で確定させる
                pageName: 'page',
                page: $page
            );

        return $paginator->through(fn (Item $item) => [
            'id'                 => (int) $item->id,
            'shop_id'            => $item->shop_id !== null ? (int) $item->shop_id : null,
            'created_by_user_id' => $item->created_by_user_id !== null ? (int) $item->created_by_user_id : null,
            'item_origin'        => (string) ($item->item_origin ?? ''),
            'name'               => (string) $item->name,
            'price'              => (int) $item->price,
            'brand'              => $item->brand,
            'condition'          => $item->condition,
            'item_image'         => $item->item_image,
            'remain'             => (int) ($item->remain ?? 0),
            'published_at'       => $item->published_at,

            // ✅ これが row に入ってないと Assembler は永遠に false
            'creator_has_shop_role' => (bool) ($item->creator_has_shop_role ?? false),
        ]);
    }
}