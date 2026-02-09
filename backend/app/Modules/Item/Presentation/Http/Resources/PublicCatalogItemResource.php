<?php

namespace App\Modules\Item\Presentation\Http\Resources;

use App\Modules\Item\ReadModel\PublicCatalog\PublicCatalogItemDto;

final class PublicCatalogItemResource
{
    public static function fromDto(PublicCatalogItemDto $dto): array
    {
        return [
            'id'    => $dto->id,
            'name'  => $dto->name,
            'price' => $dto->price,

            'brandPrimary'  => $dto->brandPrimary,
            'conditionName' => $dto->conditionName,
            'colorName'    => $dto->colorName,
            'display' => $dto->display,

            'itemImagePath' => $dto->itemImagePath
                ? asset('storage/' . $dto->itemImagePath)
                : null,

            'publishedAt'  => $dto->publishedAt->format(DATE_ATOM),
            'category' => $dto->category ?? [],
            'remain' => $dto->remain,
            'is_sold_out' => $dto->isSoldOut(),

            // ★ UI 用
            'displayType'  => $dto->displayType, // 'STAR' | 'COMET' | null
        ];
    }
}
