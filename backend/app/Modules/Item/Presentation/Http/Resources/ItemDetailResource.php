<?php

namespace App\Modules\Item\Presentation\Http\Resources;

final class ItemDetailResource
{
    /**
     * v3 FIXED
     * - 値は Repository を完全に信頼する
     * - display はメタ情報としてそのまま流す
     * - remain/is_sold_out は UI の共通判定軸として必ず返す
     */
    public static function fromReadModel(array $row): array
    {
        $remain = isset($row['remain']) ? (int)$row['remain'] : 0;

        return [
            'id'        => (int)$row['id'],
            'shop_id'   => isset($row['shop_id']) ? (int)$row['shop_id'] : null,
            'name'      => (string)$row['name'],
            'price'     => (int)$row['price'],
            'explain'   => (string)($row['explain'] ?? ''),

            // ✅ 追加（重要）
            'remain'     => $remain,
            'is_sold_out'=> $remain <= 0,

            // 画像は「/storage/xxx」形式で統一（FEが getImageUrl しても壊れない）
            'item_image' => $row['item_image'] ?? null,

            // ✅ v3 SoT（最終確定値）
            'brand'      => $row['brand'] ?? null,
            'condition'  => $row['condition'] ?? null,
            'color'      => $row['color'] ?? null,
            'category' => $row['category'] ?? [],

            // ✅ 表示メタ（由来・source 用）
            'display'    => $row['display'] ?? null,

            // 付随情報（GetItemDetailUseCase が外側で返すなら不要だが互換で残す）
            'comments'          => $row['comments'] ?? [],
            'is_favorited'      => $row['is_favorited'] ?? false,
            'favorites_count'   => $row['favorites_count'] ?? 0,
            'shop_payment_provider' => $row['shop_payment_provider'] ?? null,
        ];
    }
}