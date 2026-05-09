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

        $canManage = $shopId !== null
            && in_array($shopId, $viewerShopIds, true);

        $remain = (int) ($row['remain'] ?? 0);
        $isSoldOut = $remain <= 0;

        // =========================
        // displayType
        // 新データ:
        //   SHOP_MANAGED  => STAR
        //   USER_PERSONAL => OWN
        //
        // 旧データ救済:
        //   shop_id がある => STAR
        //   shop_id がない => OWN
        // =========================
        $itemOrigin = strtoupper(trim((string) ($row['item_origin'] ?? '')));
        $displayType = null;

        if ($itemOrigin === 'SHOP_MANAGED') {
            $displayType = 'STAR';
        } elseif ($itemOrigin === 'USER_PERSONAL') {
            $displayType = 'OWN';
        }

        // 旧データ・シーダー救済
        if ($displayType === null) {
            $displayType = $shopId !== null ? 'STAR' : 'OWN';
        }

        // if ($isFavorited) {
        //     $displayType = 'FAVORITE';
        // }

        $rawImage =
            $row['item_image']
            ?? $row['item_image_path']
            ?? $row['itemImagePath']
            ?? null;

        $itemImagePath = null;
        if (is_string($rawImage) && $rawImage !== '') {
            $normalized = '/' . ltrim($rawImage, '/');
            if (str_starts_with($normalized, '/storage/')) {
                $itemImagePath = $normalized;
            } else {
                $itemImagePath = '/storage/' . ltrim($rawImage, '/');
            }
        }

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