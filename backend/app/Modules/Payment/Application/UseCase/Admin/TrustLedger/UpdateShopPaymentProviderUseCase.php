<?php

namespace App\Modules\Payment\Application\UseCase\Admin\TrustLedger;

use Illuminate\Support\Facades\DB;

final class UpdateShopPaymentProviderUseCase
{
    public function handleRow(int $shopId, string $provider): array
    {
        $this->upsertProvider($shopId, $provider);
        return ['mode' => 'row', 'shop_id' => $shopId, 'provider' => $provider];
    }

    /**
     * @param int[] $shopIds
     */
    public function handleBulk(array $shopIds, string $provider): array
    {
        $shopIds = array_values(array_unique(array_filter($shopIds, fn ($v) => is_int($v) && $v > 0)));
        if (count($shopIds) === 0) {
            throw new \InvalidArgumentException('shop_ids empty');
        }

        DB::transaction(function () use ($shopIds, $provider) {
            foreach ($shopIds as $sid) {
                $this->upsertProvider($sid, $provider);
            }
        });

        return ['mode' => 'bulk', 'shop_ids' => $shopIds, 'provider' => $provider];
    }

    private function upsertProvider(int $shopId, string $provider): void
    {
        // shop_settings がある想定（なければ shops カラムに切替）
        DB::table('shops')->where('id',$shopId)->update(['payment_provider'=>$provider, 'updated_at'=>now()]);
    }
}