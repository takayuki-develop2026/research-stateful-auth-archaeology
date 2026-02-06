<?php

namespace App\Modules\Item\Infrastructure\Persistence\Query;

use App\Models\Good;
use App\Modules\Item\Application\Port\FavoriteItemReadPort;

final class EloquentFavoriteItemReadAdapter implements FavoriteItemReadPort
{
    public function listByUserId(int $userId): array
    {
        return Good::query()
            ->join('items', 'goods.item_id', '=', 'items.id')
            ->where('goods.user_id', $userId)
            ->whereNotNull('items.published_at')
            ->orderByDesc('goods.created_at')
            ->get([
                'items.id as id',
                'items.shop_id as shop_id',
                'items.created_by_user_id as created_by_user_id',
                'items.name as name',
                'items.price as price',
                'items.brand as brand',
                'items.condition as condition',
                'items.item_image as item_image',
                'items.remain as remain',              // ✅ 追加（超重要）
                'items.published_at as published_at',
            ])
            ->map(function ($row) {

                $remain = isset($row->remain) ? (int) $row->remain : 0;

                return [
                    'id' => (int) $row->id,
                    'shop_id' => $row->shop_id !== null ? (int) $row->shop_id : null,
                    'created_by_user_id' => $row->created_by_user_id !== null ? (int) $row->created_by_user_id : null,

                    'name' => (string) $row->name,
                    'price' => (int) $row->price,

                    'brand' => $row->brand !== null ? (string) $row->brand : null,
                    'condition' => $row->condition !== null ? (string) $row->condition : null,

                    // Assembler が /storage/ を付ける想定なので raw を返す
                    'item_image' => $row->item_image !== null ? (string) $row->item_image : null,

                    // ✅ v3 契約の核心
                    'remain' => $remain,
                    'is_sold_out' => $remain <= 0,

                    // Assembler が parse できるので string でOK
                    'published_at' => $row->published_at ? (string) $row->published_at : null,
                ];
            })
            ->all();
    }
}