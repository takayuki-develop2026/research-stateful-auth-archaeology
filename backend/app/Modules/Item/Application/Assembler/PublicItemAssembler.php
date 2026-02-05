<?php

namespace App\Modules\Item\Application\Assembler;

use App\Modules\Item\Application\Dto\Item\PublicItemDto;
use Carbon\Carbon;

final class PublicItemAssembler
{
    public static function fromReadModel(
        array $row,
        ?int $viewerUserId,
        array $viewerShopIds,
        bool $isFavorited,
        int $favoritesCount,
    ): PublicItemDto {

        $itemId = (int) ($row['id'] ?? 0);

        $shopIdRaw = $row['shop_id'] ?? null;
        $shopId = $shopIdRaw !== null ? (int) $shopIdRaw : null;

        $createdByUserIdRaw = $row['created_by_user_id'] ?? null;
        $createdByUserId = $createdByUserIdRaw !== null ? (int) $createdByUserIdRaw : null;

        $isOwner = $viewerUserId !== null
            && $createdByUserId !== null
            && $createdByUserId === $viewerUserId;

        $belongsToAnyShop = !empty($viewerShopIds);

        $canManage = $shopId !== null
            && in_array($shopId, $viewerShopIds, true);

        // remain / soldout
        $remain = (int) ($row['remain'] ?? 0);
        $isSoldOut = $remain <= 0;

        // =========================
        // displayType（固定）
        // =========================
        $displayType = null;

        // ⭐️ ショップ所属ユーザーの個人出品（shop_id=null かつ owner かつ viewer が shop所属）
        if ($shopId === null && $isOwner && $belongsToAnyShop) {
            $displayType = 'STAR';
        }
        // 💫 一般ユーザーの個人出品（shop_id=null かつ owner かつ viewer は shop非所属）
        elseif ($shopId === null && $isOwner && !$belongsToAnyShop) {
            $displayType = 'OWN';
        }

        // ❤️ FAVORITE 最優先
        if ($isFavorited) {
            $displayType = 'FAVORITE';
        }

        // 画像（複数キーを吸収して強くする）
        $rawImage =
            $row['item_image']
            ?? $row['item_image_path']
            ?? $row['itemImagePath']
            ?? null;

        $itemImagePath = is_string($rawImage) && $rawImage !== ''
            ? '/storage/' . ltrim($rawImage, '/')
            : null;

        // publishedAt（string: DATE_ATOM）
$publishedAtRaw = $row['published_at'] ?? null;
$publishedAt = null;

if ($publishedAtRaw instanceof \DateTimeInterface) {
    $publishedAt = $publishedAtRaw->format(DATE_ATOM);
} elseif (is_string($publishedAtRaw) && $publishedAtRaw !== '') {
    $publishedAt = Carbon::parse($publishedAtRaw)->format(DATE_ATOM);
}

        return new PublicItemDto(
            id: $itemId,
            name: (string) ($row['name'] ?? ''),
            price: (int) ($row['price'] ?? 0),
            itemImagePath: $itemImagePath,

            // ✅ API契約の核心
            remain: $remain,
            isSoldOut: $isSoldOut,

            brandPrimary: $row['brand'] ?? null,
            conditionName: $row['condition'] ?? null,
            colorName: $row['color'] ?? null, // 今 null 固定なら null のままでOK

            publishedAt: $publishedAt, // ✅ DateTimeInterface
            displayType: $displayType,

            isOwner: $isOwner,
            canManage: $canManage,
            isFavorited: $isFavorited,
            favoritesCount: $favoritesCount,
        );
    }
}