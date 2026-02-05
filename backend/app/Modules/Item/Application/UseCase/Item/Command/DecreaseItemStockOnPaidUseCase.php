<?php

declare(strict_types=1);

namespace App\Modules\Item\Application\UseCase\Item\Command;

use App\Modules\Item\Domain\Exception\ItemStockInconsistentException;
use App\Modules\Item\Domain\Exception\ItemStockNotEnoughException;
use Illuminate\Support\Facades\DB;

final class DecreaseItemStockOnPaidUseCase
{
    /**
     * @param array<int, array{item_id:int, quantity:int}> $rows
     */
    public function handle(int $shopId, array $rows): void
    {
        // --------
        // 0) 入力正規化（item_idごとに合算）
        // --------
        $needs = [];
        foreach ($rows as $r) {
            $itemId = (int)($r['item_id'] ?? 0);
            $qty    = (int)($r['quantity'] ?? 0);

            if ($itemId <= 0) {
                throw new ItemStockInconsistentException('item_id invalid');
            }
            if ($qty <= 0) {
                throw new ItemStockInconsistentException('quantity invalid');
            }

            $needs[$itemId] = ($needs[$itemId] ?? 0) + $qty;
        }

        if (count($needs) === 0) {
            // 減らすものが無いなら何もしない（安全）
            return;
        }

        // --------
        // 1) 競合に強い減算（同一Txで lockForUpdate → 検証 → 更新）
        // --------
        DB::transaction(function () use ($shopId, $needs) {

            $itemIds = array_keys($needs);

            // items 行ロック
            $items = DB::table('items')
                ->where('shop_id', $shopId)
                ->whereIn('id', $itemIds)
                ->lockForUpdate()
                ->select(['id', 'remain'])
                ->get();

            // 取得件数の一致チェック（存在しない商品が混ざると事故）
            if ($items->count() !== count($itemIds)) {
                $found = $items->pluck('id')->map(fn($v) => (int)$v)->all();
                $missing = array_values(array_diff($itemIds, $found));
                throw new ItemStockInconsistentException(
                    'items not found or shop mismatch: ' . json_encode($missing, JSON_UNESCAPED_UNICODE)
                );
            }

            // 不足チェック
            $notEnough = [];
            foreach ($items as $it) {
                $id = (int)$it->id;
                $remain = (int)$it->remain;
                $req = (int)$needs[$id];

                if ($remain < $req) {
                    $notEnough[] = [
                        'item_id' => $id,
                        'requested' => $req,
                        'remain' => $remain,
                    ];
                }
            }

            if (count($notEnough) > 0) {
                throw new ItemStockNotEnoughException($notEnough);
            }

            // 減算更新（個別update：行ロック済みなので安全）
            $now = now();
            foreach ($items as $it) {
                $id = (int)$it->id;
                $remain = (int)$it->remain;
                $req = (int)$needs[$id];
                $newRemain = $remain - $req;

                DB::table('items')
                    ->where('shop_id', $shopId)
                    ->where('id', $id)
                    ->update([
                        'remain' => $newRemain,
                        'updated_at' => $now,
                    ]);
            }
        });
    }
}