<?php

namespace App\Modules\Search\Presentation\Http\Resources;

use Illuminate\Http\Resources\Json\JsonResource;

final class SearchItemResource extends JsonResource
{
    public function toArray($request): array
    {
        $r = $this->resource;

        // array/object 両対応 getter
        $get = function (string $key, $default = null) use ($r) {
            if (is_array($r)) return $r[$key] ?? $default;
            if (is_object($r)) return $r->{$key} ?? $default;
            return $default;
        };

        $remain = $get('remain', null);
        $remain = is_numeric($remain) ? (int) $remain : null;

        $isSoldOut = $get('is_sold_out', null);
        if (!is_bool($isSoldOut)) {
            $isSoldOut = ($remain !== null) ? ($remain <= 0) : false;
        }

        return [
            'id'      => $get('id'),
            'shop_id' => $get('shop_id'),
            'name'    => $get('name'),

            // 既存互換：snake のまま返す（フロントが吸収できてる）
            'item_image_path' => $get('item_image_path'),

            'price' => [
                'amount'   => $get('price_amount'),
                'currency' => $get('price_currency'),
            ],

            'created_at' => $get('created_at'),

            // ✅ 追加（超重要）
            'remain'      => $remain,
            'is_sold_out' => (bool) $isSoldOut,
        ];
    }
}
