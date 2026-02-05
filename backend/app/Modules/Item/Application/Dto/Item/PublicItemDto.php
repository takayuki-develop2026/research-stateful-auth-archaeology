<?php

namespace App\Modules\Item\Application\Dto\Item;

final class PublicItemDto
{
    public function __construct(
        public int $id,
        public string $name,
        public int $price,
        public ?string $itemImagePath,
        public readonly int $remain,
        public readonly bool $isSoldOut,
        public ?string $brandPrimary,
        public ?string $conditionName,
        public ?string $colorName,
        public ?string $publishedAt,

        /**
         * 表示制御
         * null | STAR | OWN | FAVORITE
         */
        public ?string $displayType,

        /**
         * 権限・状態
         */
        public bool $isOwner,
        public bool $canManage,
        public bool $isFavorited,     // ★ 追加
        public int $favoritesCount,  // ★ 追加
    ) {
    }

    public function toArray(): array
    {
        return [
            'id'              => $this->id,
            'name'            => $this->name,
            'price'           => $this->price,
            'itemImagePath'   => $this->itemImagePath,
            'remain' => $this->remain,
            'is_sold_out' => $this->isSoldOut,
            'brandPrimary'    => $this->brandPrimary,
            'conditionName'   => $this->conditionName,
            'colorName'       => $this->colorName,
            'publishedAt'     => $this->publishedAt,
            'displayType'     => $this->displayType,
            'isOwner'         => $this->isOwner,
            'canManage'       => $this->canManage,
            'isFavorited'     => $this->isFavorited,
            'favoritesCount'  => $this->favoritesCount,
        ];
    }
}