<?php

namespace App\Modules\Item\Infrastructure\Persistence\Query;

use App\Models\Item;
use Illuminate\Support\Facades\DB;
use App\Modules\Item\Infrastructure\Persistence\Query\AnalysisResultReadRepository;

final class ItemReadRepository
{
    public function __construct(
        private readonly ItemEntityTagReadRepository $tagRepo,
        private readonly AnalysisResultReadRepository $analysisRepo,
    ) {
    }

    public function findWithDisplayBrand(int $itemId)
    {
        return Item::query()
            ->leftJoin('item_entities', 'items.id', '=', 'item_entities.item_id')
            ->leftJoin('brand_entities', 'item_entities.brand_entity_id', '=', 'brand_entities.id')
            ->where('items.id', $itemId)
            ->select([
                'items.*',
                DB::raw('COALESCE(brand_entities.canonical_name, items.brand) as display_brand'),
            ])
            ->first();
    }

    public function paginateWithDisplayBrand(int $limit, int $page)
    {
        return Item::query()
            ->leftJoin('item_entities', function ($join) {
                $join->on('items.id', '=', 'item_entities.item_id')
                    ->where('item_entities.is_latest', true);
            })
            ->leftJoin('brand_entities', 'item_entities.brand_entity_id', '=', 'brand_entities.id')
            ->select([
                'items.*',
                DB::raw('COALESCE(brand_entities.canonical_name, items.brand) as display_brand'),
            ])
            ->paginate($limit, ['*'], 'page', $page);
    }

    /**
     * 商品詳細（v3固定）
     * 優先順位：
     * 1) human_confirmed（存在するなら必ずこれ）
     * 2) is_latest=true の entity（ai_provisional 等）
     * 3) analysis_results（human_confirmed が無い時のみ）
     * 4) items（raw）
     */
    public function findWithDisplayEntities(int $itemId): ?array
{
    $row = DB::table('items as i')
        ->leftJoin('shops as s', 's.id', '=', 'i.shop_id')
        ->where('i.id', $itemId)
        ->select([
            'i.id',
            'i.shop_id',
            'i.name',
            'i.price',
            'i.explain',
            'i.remain',
            'i.item_image',
            'i.brand',
            'i.condition',
            's.payment_provider as shop_payment_provider',
        ])
        ->first();

    if (! $row) return null;

    // 既存ロジックはそのまま（$item を $row に置き換える）
    $entity = $this->pickBestEntityRow($row->id);

    if ($entity !== null) {
        $display = [
            'brand' => ['name' => $entity->brand_name, 'source' => $entity->source, 'is_latest' => true],
            'condition' => ['name' => $entity->condition_name, 'source' => $entity->source, 'is_latest' => true],
            'color' => ['name' => null,'source' => 'raw','is_latest' => false,],
        ];
    } elseif ($analysis = $this->analysisRepo->findLatestActiveByItemId($row->id)) {
        $display = [
            'brand' => [...($analysis['brand'] ?? ['name' => null]), 'is_latest' => false],
            'condition' => [...($analysis['condition'] ?? ['name' => null]), 'is_latest' => false],
            'color' => [...($analysis['color'] ?? ['name' => null]), 'is_latest' => false],
        ];
    } else {
        $display = [
            'brand' => ['name' => $row->brand, 'source' => 'raw', 'is_latest' => false],
            'condition' => ['name' => $row->condition, 'source' => 'raw', 'is_latest' => false],
            'color' => ['name' => null, 'source' => 'raw', 'is_latest' => false],
        ];
    }

    return [
        'id'        => (int) $row->id,
        'shop_id'   => (int) $row->shop_id,
        'name'      => (string) $row->name,
        'price'     => (int) $row->price,
        'explain'   => (string) $row->explain,
        'remain'    => (int) $row->remain,
        'item_image'=> $row->item_image,

        'brand'     => $display['brand']['name'],
        'condition' => $display['condition']['name'],
        'color'     => $display['color']['name'],
        'display'   => $display,

        // ★追加：shop の決済プロバイダ
        'shop_payment_provider' => $row->shop_payment_provider ?? null,
    ];
}

    /**
     * v3固定：entity選択（最重要）
     * - human_confirmed が 1件でもあるなら、それを返す（is_latest を信用しない）
     * - ない場合だけ is_latest=true の entity を返す
     *
     * 返却は canonical_name JOIN済みの行（brand_name/condition_name/color_name を必ず持つ）
     */
    private function pickBestEntityRow(int $itemId): ?object
    {
        // A) human_confirmed を最優先（is_latest が壊れてても拾う）
        $human = DB::table('item_entities as ie')
            ->leftJoin('brand_entities as be', 'ie.brand_entity_id', '=', 'be.id')
            ->leftJoin('condition_entities as ce', 'ie.condition_entity_id', '=', 'ce.id')
            ->leftJoin('color_entities as coe', 'ie.color_entity_id', '=', 'coe.id')
            ->where('ie.item_id', $itemId)
            ->where('ie.source', 'human_confirmed')
            ->orderByDesc('ie.id')
            ->select([
                'ie.id',
                'ie.source',
                'be.canonical_name as brand_name',
                'ce.canonical_name as condition_name',
                'coe.canonical_name as color_name',
            ])
            ->first();

        if ($human !== null) {
            return $human;
        }

        // B) human_confirmed が無い時だけ is_latest を採用
        return DB::table('item_entities as ie')
            ->leftJoin('brand_entities as be', 'ie.brand_entity_id', '=', 'be.id')
            ->leftJoin('condition_entities as ce', 'ie.condition_entity_id', '=', 'ce.id')
            ->leftJoin('color_entities as coe', 'ie.color_entity_id', '=', 'coe.id')
            ->where('ie.item_id', $itemId)
            ->where('ie.is_latest', true)
            ->orderByRaw("
                CASE ie.source
                    WHEN 'ai_provisional' THEN 1
                    ELSE 2
                END
            ")
            ->orderByDesc('ie.id')
            ->select([
                'ie.id',
                'ie.source',
                'be.canonical_name as brand_name',
                'ce.canonical_name as condition_name',
                'coe.canonical_name as color_name',
            ])
            ->first();
    }

    /**
     * 一覧（軽量）
     * - ここは is_latest 前提（ApplyConfirmedDecision 側で is_latest を保証する）
     * - 一覧で human_confirmed を完全優先したいなら、後で View / subquery 化する（今は重くしない）
     */
    public function paginateWithDisplayEntities(int $limit, int $page)
    {
        return Item::query()
            ->leftJoin('item_entities as ie', function ($join) {
                $join->on('items.id', '=', 'ie.item_id')
                    ->where('ie.is_latest', true);
            })
            ->leftJoin('brand_entities as be', 'ie.brand_entity_id', '=', 'be.id')
            ->leftJoin('condition_entities as ce', 'ie.condition_entity_id', '=', 'ce.id')
            ->leftJoin('color_entities as coe', 'ie.color_entity_id', '=', 'coe.id')
            ->select([
                'items.id',
                'items.name',
                'items.price',
                'items.item_image',
                'be.canonical_name as brand_primary',
                'ce.canonical_name as condition_name',
                'coe.canonical_name as color_name',
                'ie.source as entity_source',
            ])
            ->paginate($limit, ['*'], 'page', $page)
            ->through(function ($row) {
                return [
                    'id'        => $row->id,
                    'name'      => $row->name,
                    'price'     => $row->price,
                    'brand'     => $row->brand_primary,
                    'condition' => $row->condition_name,
                    'color'     => $row->color_name,
                    'meta'      => [
                        'source' => $row->entity_source,
                    ],
                    'item_image' => $row->item_image
                        ? asset('storage/' . $row->item_image)
                        : null,
                ];
            });
    }

    public function findWithDisplayEntitiesAndTags(
        int $itemId,
        ItemEntityTagReadRepository $tagRepo
    ): ?array {
        $item = $this->findWithDisplayEntities($itemId);
        if (! $item) {
            return null;
        }

        return [
            'item' => $item,
            'tags' => $tagRepo->getGroupedByItemId($itemId),
        ];
    }

    /**
     * NOTE: Repository の責務ではないので削除推奨（現状維持なら残しても良いが動かない）
     * $this->toArray() はこのクラスに存在しないので実行すると落ちます。
     */
    public function withFavorite(bool $isFavorited): array
    {
        return [
            'isFavorited' => $isFavorited,
        ];
    }
}