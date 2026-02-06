<?php

namespace App\Modules\Item\Infrastructure\Persistence\Query;

use App\Models\Item;
use App\Modules\Item\Application\Query\PublicCatalogQueryService;
use Illuminate\Contracts\Pagination\LengthAwarePaginator;

final class EloquentPublicCatalogQueryService implements PublicCatalogQueryService
{
    public function paginate(
        int $limit,
        int $page,
        ?string $keyword,
        array $excludeShopIds = []
    ): LengthAwarePaginator {
        $query = Item::query()
            ->whereNotNull('published_at');

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
                columns: [
                    'id',
                    'shop_id',
                    'created_by_user_id',
                    'item_origin',
                    'name',
                    'price',
                    'brand',
                    'condition',
                    'item_image',
                    'remain',        // ✅ remain を必ず返す（超重要）
                    'published_at',
                ],
                pageName: 'page',
                page: $page
            );

        /**
         * Eloquent Model → ReadModel(array) に正規化
         */
        return $paginator->through(
            fn (Item $item) => [
                'id'                 => (int) $item->id,
                'shop_id'            => $item->shop_id !== null ? (int) $item->shop_id : null,
                'created_by_user_id' => $item->created_by_user_id !== null ? (int) $item->created_by_user_id : null,
                'item_origin'        => (string) $item->item_origin,
                'name'               => (string) $item->name,
                'price'              => (int) $item->price,
                'brand'              => $item->brand,
                'condition'          => $item->condition,
                'item_image'         => $item->item_image,

                // ✅ remain
                'remain'             => (int) ($item->remain ?? 0),

                // ✅ DateTimeInterface（Carbon）で保持（AssemblerでDATE_ATOM文字列へ）
                'published_at'       => $item->published_at,
            ]
        );
    }
}