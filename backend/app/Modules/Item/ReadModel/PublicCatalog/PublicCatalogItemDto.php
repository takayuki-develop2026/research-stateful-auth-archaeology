<?php

namespace App\Modules\Item\ReadModel\PublicCatalog;

final class PublicCatalogItemDto
{
    public function __construct(
        public readonly int $id,
        public readonly string $name,
        public readonly int $price,
        public readonly int $remain,

        public readonly ?string $brandPrimary,
        public readonly ?string $conditionName,
        public readonly ?string $colorName,

        // ✅ 追加：JSON category（例: ["PC","ストレージ","HDD"]）
        public readonly ?array $category,

        public readonly ?string $itemImagePath,
        public readonly \DateTimeInterface $publishedAt,
        public readonly ?string $itemOrigin,
        public readonly ?string $displayType, // 'STAR' | 'COMET' | null
        public readonly ?array $display,
    ) {
    }

    public function isSoldOut(): bool
    {
        return $this->remain <= 0;
    }

    public function toArray(): array
    {
        return [
            'id' => $this->id,
            'name' => $this->name,
            'price' => $this->price,
            'remain' => $this->remain,
            'is_sold_out' => $this->isSoldOut(),

            'brandPrimary' => $this->brandPrimary,
            'conditionName' => $this->conditionName,
            'colorName' => $this->colorName,

            // ✅ 追加
            'category' => $this->category,

            'itemImagePath' => $this->itemImagePath,
            'publishedAt' => $this->publishedAt->format('Y-m-d H:i:s'),
            'item_origin' => $this->itemOrigin,
            'displayType' => $this->displayType,
            'display' => $this->display,
        ];
    }
}