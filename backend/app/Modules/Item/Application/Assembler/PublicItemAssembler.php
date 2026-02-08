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

$itemOrigin = (string) ($row['item_origin'] ?? '');
$creatorHasShopRole = (bool) ($row['creator_has_shop_role'] ?? false);

$displayType = null;

if ($itemOrigin === 'USER_PERSONAL') {
    $displayType = $creatorHasShopRole ? 'STAR' : 'OWN';
}

// ✅ 個人出品判定（shop_idに依存しない救済込み）
$isPersonalListing =
    ($shopId === null)
    || in_array($itemOrigin, [
        'personal',
        'individual',
        'user',
        'user_personal',
        'personal_user',
    ], true);

// ✅ 個人出品だけマーク付与：一般=💫 / shop関係=⭐️
// ✅ ショップ出品はマーク無し（null）
if ($isPersonalListing) {
    $displayType = $creatorHasShopRole ? 'STAR' : 'OWN';
}

        // ❤️ フロントで、FAVORITE 最優先（既存ロジック維持）
        // if ($isFavorited) {
        //     $displayType = 'FAVORITE';
        // }

        // 画像（複数キーを吸収して強くする）
        $rawImage =
            $row['item_image']
            ?? $row['item_image_path']
            ?? $row['itemImagePath']
            ?? null;

        // ここは現行方針に合わせて強制 /storage プレフィックス
        // item_image が "item_images/xxx.png" の場合 → "/storage/item_images/xxx.png"
        // すでに "/storage/..." が入ってきても二重にならないようガード
        $itemImagePath = null;
        if (is_string($rawImage) && $rawImage !== '') {
            $normalized = '/' . ltrim($rawImage, '/');
            if (str_starts_with($normalized, '/storage/')) {
                $itemImagePath = $normalized;
            } else {
                $itemImagePath = '/storage/' . ltrim($rawImage, '/');
            }
        }

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
            colorName: $row['color'] ?? null,

            publishedAt: $publishedAt,
            displayType: $displayType,

            isOwner: $isOwner,
            canManage: $canManage,
            isFavorited: $isFavorited,
            favoritesCount: $favoritesCount,
        );
    }
}