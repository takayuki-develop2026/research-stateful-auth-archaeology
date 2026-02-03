<?php

namespace App\Modules\Item\Domain\Entity;

use DateTimeImmutable;

use App\Modules\Item\Domain\ValueObject\{
    ItemDraftId,
    SellerId,
    ItemName,
    Money,
    BrandName,
    ItemStatus,
    CategoryList,
    ItemImagePath,
    StockCount,
    ItemOrigin
};

final class ItemDraft
{
    private ?ItemImagePath $itemImage;

    private function __construct(
        private ItemDraftId $id,
        private SellerId $sellerId,
        private ?int $shopId,              // ✅ 追加：確定した事実
        private ItemName $name,
        private Money $price,
        private ?BrandName $brandRaw,
        private ItemStatus $status,
        private string $explain,
        private string $condition,
        private CategoryList $category,
        private StockCount $remain,
        ?ItemImagePath $itemImage,
    ) {
        $this->itemImage = $itemImage;
    }

    /* =========================
       Factory（新規 Draft 作成）
    ========================= */
    public static function create(
        ItemDraftId $id,
        SellerId $sellerId,
        ?int $shopId,
        ItemName $name,
        Money $price,
        ?BrandName $brandRaw,
        ?string $explain,
        ?string $condition,
        ?array $category,
    ): self {
        return new self(
            id: $id,
            sellerId: $sellerId,
            shopId: $shopId,
            name: $name,
            price: $price,
            brandRaw: $brandRaw,
            status: ItemStatus::DRAFT,
            explain: $explain ?? '',
            condition: $condition ?? '',
            category: new CategoryList($category ?? []),
            remain: new StockCount(1),
            itemImage: null,
        );
    }

    /* =========================
       Repository 再構築用
    ========================= */
    public static function reconstruct(
        ItemDraftId $id,
        SellerId $sellerId,
        ?int $shopId,
        ItemName $name,
        Money $price,
        ?BrandName $brandRaw,
        ItemStatus $status,
        string $explain,
        string $condition,
        array $category,
        StockCount $remain,
        ?ItemImagePath $itemImage,
    ): self {
        return new self(
            id: $id,
            sellerId: $sellerId,
            shopId: $shopId,
            name: $name,
            price: $price,
            brandRaw: $brandRaw,
            status: $status,
            explain: $explain,
            condition: $condition,
            category: new CategoryList($category),
            remain: $remain,
            itemImage: $itemImage,
        );
    }


    /* =========================
       Getter
    ========================= */

    public function id(): ItemDraftId
    {
        return $this->id;
    }
    public function sellerId(): SellerId
    {
        return $this->sellerId;
    }
    public function shopId(): ?int
    {
        return $this->shopId;
    }

    public function itemOrigin(): ItemOrigin
    {
        return ItemOrigin::fromSellerId($this->sellerId);
    }

    public function name(): ItemName
    {
        return $this->name;
    }
    public function price(): Money
    {
        return $this->price;
    }
    public function brand(): ?BrandName
    {
        return $this->brandRaw;
    }
    public function status(): ItemStatus
    {
        return $this->status;
    }
    public function explain(): string
    {
        return $this->explain;
    }
    public function condition(): string
    {
        return $this->condition;
    }
    public function category(): CategoryList
    {
        return $this->category;
    }
    public function remain(): StockCount
    {
        return $this->remain;
    }

    /* =========================
       Image
    ========================= */

    public function itemImage(): ?ItemImagePath
    {
        return $this->itemImage;
    }

    public function attachImage(ItemImagePath $path): void
    {
        $this->itemImage = $path;
    }

    public function hasImage(): bool
    {
        return $this->itemImage !== null;
    }

    /* =========================
       Publish Rule
    ========================= */

    public function isPublishableV1(): bool
    {
        return $this->status === ItemStatus::DRAFT
            && $this->hasImage();
    }

    public function markPublished(): void
    {
        $this->status = ItemStatus::PUBLISHED;
    }

    /**
 * v3 固定：
 * Draft に入力された「人間の生値」を返す
 */
public function brandRaw(): ?string
{
    return $this->brandRaw?->value();
}

public function conditionRaw(): ?string
{
    return $this->condition !== '' ? $this->condition : null;
}

/**
 * color は Draft に存在しない可能性があるため
 * v3 では null 固定で安全に返す
 */
public function colorRaw(): ?string
{
    return null;
}

public function color(): ?string
{
    return null;
}
}
