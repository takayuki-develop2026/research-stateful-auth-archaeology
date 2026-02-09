<?php

namespace App\Modules\Item\Infrastructure\Persistence\Query;

use App\Models\Item;
use Illuminate\Support\Facades\DB;

final class ItemReadRepository
{
    public function __construct(
        private readonly ItemEntityTagReadRepository $tagRepo,
        private readonly AnalysisResultReadRepository $analysisRepo,
    ) {
    }

    /**
     * 内部用（必要なら維持）
     * - public API で使うなら items.* は絞ること
     */
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

    /**
     * 内部用（必要なら維持）
     */
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
     * 商品詳細（v3固定 / "落ちない" 確定版）
     * 優先順位：
     * 1) human_confirmed（存在するなら必ずこれ）
     * 2) is_latest=true の entity（ai_provisional 等）
     * 3) analysis_results（human_confirmed が無い時のみ）
     * 4) items（raw）
     *
     * 注意：
     * - items に color カラムが無い前提なので、raw fallback の color は null 固定
     * - item_entities.color_entity_id が無い可能性があるので、color_entities join は一切しない（NULL固定）
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
                'i.category',
                's.payment_provider as shop_payment_provider',
            ])
            ->first();

        if (!$row) {
            return null;
        }

        $category = $row->category ?? null;

// DB::table は JSON を string で返すことがある
if (is_string($category) && $category !== '') {
    $decoded = json_decode($category, true);

    // 1) 正常：JSON配列文字列 -> array
    if (is_array($decoded)) {
        $category = $decoded;
    }
    // 2) 二重：JSON文字列の中に配列文字列が入ってる -> もう一回 decode
    elseif (is_string($decoded) && $decoded !== '') {
        $decoded2 = json_decode($decoded, true);
        $category = is_array($decoded2) ? $decoded2 : [$decoded];
    }
    // 3) JSONじゃない普通の文字列 -> 区切って配列化（保険）
    else {
        $parts = preg_split('/[|\/,\x{3001}\x{30fb}]+/u', $category) ?: [];
        $parts = array_values(array_filter(array_map('trim', $parts), fn($v) => $v !== ''));
        $category = $parts;
    }
}

// 最終保証：必ず配列
if (!is_array($category)) {
    $category = [];
}

        $entity = $this->pickBestEntityRow((int) $row->id);

        if ($entity !== null) {
            // ✅ entity 採用（color はスキーマ不確定なので NULL固定）
            $display = [
  'brand' => [
    'name' => $entity->brand_name ?? null,
    'source' => $entity->source,
    'is_latest' => true,
  ],
  'condition' => [
    'name' => $entity->condition_name ?? null,
    'source' => $entity->source,
    'is_latest' => true,
  ],
  'color' => [
    'name' => $entity->color_name ?? null, // ★復活
    'source' => $entity->source,
    'is_latest' => true,
  ],
];
        } elseif ($analysis = $this->analysisRepo->findLatestActiveByItemId((int) $row->id)) {
            // ✅ analysis_results（あれば color も反映できる）
            $display = [
                'brand' => array_merge(($analysis['brand'] ?? ['name' => null]), ['is_latest' => false]),
                'condition' => array_merge(($analysis['condition'] ?? ['name' => null]), ['is_latest' => false]),
                'color' => array_merge(($analysis['color'] ?? ['name' => null]), ['is_latest' => false]),
            ];
        } else {
            // ✅ raw fallback（itemsにcolorが無いので null 固定）
            $display = [
                'brand' => [
                    'name' => $row->brand ?? null,
                    'source' => 'raw',
                    'is_latest' => false,
                ],
                'condition' => [
                    'name' => $row->condition ?? null,
                    'source' => 'raw',
                    'is_latest' => false,
                ],
                'color' => [
                    'name' => null,
                    'source' => 'raw',
                    'is_latest' => false,
                ],
            ];
        }

        // item_image を /storage/ パスに寄せる（public返却の標準）
        $rawImage = $row->item_image ?? null;
        $itemImagePath = (is_string($rawImage) && $rawImage !== '')
            ? '/storage/' . ltrim($rawImage, '/')
            : null;

        $shopId = $row->shop_id !== null ? (int) $row->shop_id : null;

        return [
            'id'        => (int) $row->id,
            'shop_id'   => $shopId, // ✅ null を 0 にしない
            'name'      => (string) $row->name,
            'price'     => (int) $row->price,
            'explain'   => (string) ($row->explain ?? ''),
            'remain'    => (int) ($row->remain ?? 0),
            'item_image'=> $itemImagePath,

            // v3 SoT（最終確定値：displayから採用）
            'brand'     => $display['brand']['name'] ?? null,
            'condition' => $display['condition']['name'] ?? null,
            'color'     => $display['color']['name'] ?? null,
            'category'  => $category,
            'display'   => $display,

            // shop の決済プロバイダ
            'shop_payment_provider' => $row->shop_payment_provider ?? null,
        ];
    }

    /**
     * v3固定：entity選択（最重要 / "落ちない" 確定版）
     * - human_confirmed が 1件でもあるなら、それを返す（is_latest を信用しない）
     * - ない場合だけ is_latest=true の entity を返す
     *
     * 注意：
     * - item_entities.color_entity_id が無い可能性があるため、color_entities join はしない（NULL固定）
     */
    private function pickBestEntityRow(int $itemId): ?object
{
    $human = DB::table('item_entities as ie')
        ->leftJoin('brand_entities as be', 'ie.brand_entity_id', '=', 'be.id')
        ->leftJoin('condition_entities as cde', 'ie.condition_entity_id', '=', 'cde.id')
        ->leftJoin('color_entities as cle', 'ie.color_entity_id', '=', 'cle.id') // ★追加
        ->where('ie.item_id', $itemId)
        ->where('ie.source', 'human_confirmed')
        ->orderByDesc('ie.id')
        ->select([
            'ie.id',
            'ie.source',
            'be.canonical_name as brand_name',
            'cde.canonical_name as condition_name',
            'cle.canonical_name as color_name', // ★NULL固定をやめる
        ])
        ->first();

    if ($human !== null) {
        return $human;
    }

    return DB::table('item_entities as ie')
        ->leftJoin('brand_entities as be', 'ie.brand_entity_id', '=', 'be.id')
        ->leftJoin('condition_entities as cde', 'ie.condition_entity_id', '=', 'cde.id')
        ->leftJoin('color_entities as cle', 'ie.color_entity_id', '=', 'cle.id') // ★追加
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
            'cde.canonical_name as condition_name',
            'cle.canonical_name as color_name', // ★追加
        ])
        ->first();
}

    /**
     * 一覧（軽量 / "落ちない" 確定版）
     * - is_latest 前提（ApplyConfirmedDecision 側で保証）
     * - color はスキーマ不確定のため NULL固定（後で復活）
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
            ->select([
                'items.id',
                'items.shop_id',
                'items.created_by_user_id',
                'items.name',
                'items.price',
                'items.remain',
                'items.item_image',
                'be.canonical_name as brand_primary',
                'ce.canonical_name as condition_name',
                DB::raw('NULL as color_name'),
                'ie.source as entity_source',
            ])
            ->paginate($limit, ['*'], 'page', $page)
            ->through(function ($row) {
                $rawImage = $row->item_image ?? null;
                $itemImagePath = (is_string($rawImage) && $rawImage !== '')
                    ? '/storage/' . ltrim($rawImage, '/')
                    : null;

                $remain = isset($row->remain) ? (int) $row->remain : 0;

                return [
                    'id'        => (int) $row->id,
                    'shop_id'   => $row->shop_id !== null ? (int) $row->shop_id : null,
                    'created_by_user_id' => $row->created_by_user_id !== null ? (int) $row->created_by_user_id : null,
                    'name'      => (string) $row->name,
                    'price'     => (int) $row->price,

                    // ✅ 契約（必須）
                    'remain'    => $remain,
                    'is_sold_out' => $remain <= 0,

                    'brand'     => $row->brand_primary ?? null,
                    'condition' => $row->condition_name ?? null,
                    'color'     => null, // ✅ fixed

                    'meta'      => [
                        'source' => $row->entity_source,
                    ],

                    // PublicItemAssembler が吸収するキー：item_image
                    'item_image' => $itemImagePath,
                    'published_at' => null, // 必要なら select に items.published_at を追加して埋める
                ];
            });
    }

    public function findWithDisplayEntitiesAndTags(
        int $itemId,
        ItemEntityTagReadRepository $tagRepo
    ): ?array {
        $item = $this->findWithDisplayEntities($itemId);
        if (!$item) {
            return null;
        }

        return [
            'item' => $item,
            'tags' => $tagRepo->getGroupedByItemId($itemId),
        ];
    }

    /**
     * NOTE: Repository の責務ではないので削除推奨
     * - このメソッドは ItemReadRepository の状態を表さず、返却が中途半端
     */
    public function withFavorite(bool $isFavorited): array
    {
        return [
            'isFavorited' => $isFavorited,
        ];
    }
}