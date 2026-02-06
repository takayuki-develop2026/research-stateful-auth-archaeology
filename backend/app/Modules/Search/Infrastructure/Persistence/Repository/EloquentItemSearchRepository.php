<?php

namespace App\Modules\Search\Infrastructure\Persistence\Repository;

use App\Modules\Search\Domain\Collection\SearchResultItems;
use App\Modules\Search\Domain\Criteria\ItemSearchCriteria;
use App\Modules\Search\Domain\Repository\ItemSearchRepository;
use Illuminate\Support\Facades\DB;

final class EloquentItemSearchRepository implements ItemSearchRepository
{
    public function search(ItemSearchCriteria $criteria): SearchResultItems
    {
        $q = DB::table('items');

        // 公開状態（SoT: published_at）
        if ($criteria->onlyPublished) {
            $q->whereNotNull('items.published_at');
        }

        // shop 絞り込み
        if ($criteria->shopId !== null) {
            $q->where('items.shop_id', $criteria->shopId);
        }

        // keyword 検索
        if ($criteria->keyword !== null && trim($criteria->keyword) !== '') {
            $kw = trim($criteria->keyword);

            $q->where(function ($sub) use ($kw) {
                $sub->where('items.name', 'like', '%' . $kw . '%')
                    ->orWhere('items.explain', 'like', '%' . $kw . '%')
                    ->orWhere('items.brand', 'like', '%' . $kw . '%');
            });
        }

        // total（ページング前）
        $total = (clone $q)->count();

        // sort
        $sort = $criteria->sort;
        if ($sort !== null) {
            $allowed = ['created_at', 'price', 'id'];
            $field = in_array($sort->field, $allowed, true) ? $sort->field : 'created_at';
            $dir = strtolower($sort->direction) === 'asc' ? 'asc' : 'desc';
            $q->orderBy('items.' . $field, $dir);
        } else {
            $q->orderBy('items.created_at', 'desc');
        }

        // select（✅ remain を含める）
        $q->select([
            'items.id',
            'items.shop_id',
            'items.name',
            'items.price',
            'items.price_currency',
            'items.item_image',
            'items.remain',
            'items.created_at',
        ]);

        // pagination
        if ($criteria->pagination !== null) {
            $q->offset($criteria->pagination->offset())
              ->limit($criteria->pagination->perPage);
        }

        $rows = $q->get();

        $items = $rows->map(function ($r) {

            $shopId = $r->shop_id !== null ? (int) $r->shop_id : null;

            $rawImage = $r->item_image ?? null;
            $itemImagePath = (is_string($rawImage) && $rawImage !== '')
                ? '/storage/' . ltrim($rawImage, '/')
                : null;

            $remain = isset($r->remain) ? (int) $r->remain : 0;

            return [
                'id'              => (int) $r->id,
                'shop_id'         => $shopId,
                'name'            => (string) $r->name,

                // 既存互換（price object）
                'price_amount'    => (int) $r->price,
                'price_currency'  => (string) $r->price_currency,

                // 既存互換（snake）
                'item_image_path' => $itemImagePath,
                'created_at'      => $r->created_at,

                // ✅ v3契約の核心
                'remain'          => $remain,
                'is_sold_out'     => $remain <= 0,
            ];
        })->all();

        return new SearchResultItems($items, $total);
    }
}