<?php

namespace App\Modules\Item\Domain\Service;

use App\Modules\Item\Domain\ValueObject\SellerId;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use App\Modules\Item\Domain\ValueObject\SellerType;

final class SellerAuthorizationService
{
    public function canOperate(
        SellerId $sellerId,
        AuthPrincipal $principal,
    ): bool {
        return match ($sellerId->type()) {

            // 個人出品
            SellerType::INDIVIDUAL =>
                $sellerId->id() !== null
                && $principal->userId() === $sellerId->id(),

            // 店舗出品
            SellerType::SHOP =>
                $this->canOperateShop($sellerId, $principal),
        };
    }

    private function canOperateShop(
        SellerId $sellerId,
        AuthPrincipal $principal,
    ): bool {
        // Draft フェーズ（shop:managed）※ id が未確定なら「何かしら shop 権限あるか」
        if ($sellerId->id() === null) {
            return !empty($principal->shopIds()); // ✅ メソッド呼び出し
        }

        // Publish 後（shop:ID）
        return in_array(
            (int) $sellerId->id(),
            $principal->shopIds(), // ✅ メソッド呼び出し
            true
        );
    }
}