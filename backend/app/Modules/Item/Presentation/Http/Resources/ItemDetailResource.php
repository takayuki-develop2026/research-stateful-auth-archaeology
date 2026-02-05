<?php

namespace App\Modules\Item\Presentation\Http\Resources;

final class ItemDetailResource
{
    /**
     * v3 FIXED
     * - 値は Repository を完全に信頼する
     * - display はメタ情報としてそのまま流す
     */
    public static function fromReadModel(array $row): array
    {
        return [
            'id'        => $row['id'],
            'shop_id'   => $row['shop_id'] ?? null,
            'name'      => $row['name'],
            'price'     => $row['price'],
            'explain'   => $row['explain'],
            'remain'    => $row['remain'],
            'item_image'=> $row['item_image'],

            // ✅ v3 SoT（最終確定値）
            'brand'     => $row['brand'],
            'condition' => $row['condition'],
            'color'     => $row['color'],

            // ✅ 表示メタ（由来・source 用）
            'display'   => $row['display'],

            // 付随情報
            'comments'         => $row['comments'] ?? [],
            'is_favorited'     => $row['is_favorited'] ?? false,
            'favorites_count' => $row['favorites_count'] ?? 0,
            'shop_payment_provider' => $row['shop_payment_provider'] ?? null,
        ];
    }
}