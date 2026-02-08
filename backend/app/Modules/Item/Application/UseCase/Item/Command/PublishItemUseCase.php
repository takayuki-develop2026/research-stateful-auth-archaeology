<?php

namespace App\Modules\Item\Application\UseCase\Item\Command;

use App\Modules\Item\Application\Dto\Item\PublishItemInput;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use App\Modules\Item\Domain\Repository\ItemDraftRepository;
use App\Modules\Item\Domain\Repository\ItemRepository;
use App\Modules\Item\Domain\ValueObject\ItemImagePath;
use App\Modules\Item\Domain\Entity\Item;
use App\Modules\Item\Domain\Service\SellerAuthorizationService;
use App\Modules\Item\Domain\ValueObject\StockCount;
use App\Modules\Item\Domain\ValueObject\SellerType;
use App\Modules\Item\Domain\ValueObject\ItemOrigin as ItemOriginVO;
use Illuminate\Support\Facades\Event;
use Illuminate\Support\Facades\Storage;
use Illuminate\Support\Facades\DB;
use App\Modules\Item\Application\Event\ItemImported;
use DomainException;

final class PublishItemUseCase
{
    public function __construct(
        private ItemDraftRepository $draftRepository,
        private ItemRepository $itemRepository,
        private SellerAuthorizationService $sellerAuth,
    ) {
    }

    public function execute(
        PublishItemInput $input,
        AuthPrincipal $principal,
        ?int $tenantId,
    ): void {
        $itemId = null;
        $rawText = null;

        DB::transaction(function () use ($input, $principal, &$itemId, &$rawText) {

            $draft = $this->draftRepository->findById($input->draftId);

            if (! $draft || ! $draft->isPublishableV1()) {
                throw new DomainException('Draft is not publishable');
            }

            $sellerId = $draft->sellerId();

            /**
             * ✅ B方針：個人は shop_id = null 固定
             * - SHOP: sellerId が shop:2 等ならそれを採用。shop:managed 等で id が無いなら input.shopId 必須
             * - INDIVIDUAL: 常に null
             */
            $shopId = match ($sellerId->type()) {
                SellerType::SHOP => $sellerId->id() ?? $input->shopId,
                SellerType::INDIVIDUAL => null,
            };

            // ✅ SHOP のときだけ shop_id を必須化
            if ($sellerId->type() === SellerType::SHOP && $shopId === null) {
                throw new DomainException('shop_id is required to publish item');
            }

            // ✅ SHOP のときだけ mismatch 判定
            if (
                $sellerId->type() === SellerType::SHOP &&
                $sellerId->id() !== null &&
                $sellerId->id() !== $shopId
            ) {
                throw new DomainException('shop_id mismatch');
            }

            $price = $draft->price();
            if ($price === null) {
                throw new DomainException('price is required to publish');
            }

            // 画像昇格
            $itemImage = null;
            if ($draftImageVO = $draft->itemImage()) {
                $draftImagePath = $draftImageVO->value();
                $itemImagePath = str_replace('item_drafts/', 'item_images/', $draftImagePath);

                if (! Storage::disk('public')->exists($itemImagePath)) {
                    Storage::disk('public')->copy($draftImagePath, $itemImagePath);
                }

                $itemImage = ItemImagePath::fromRaw($itemImagePath);
            }

            // ✅ Item 作成（B方針：shopId nullable）
            $item = Item::createNew(
                itemOrigin: ItemOriginVO::from(
                    $sellerId->type() === SellerType::SHOP
                        ? ItemOriginVO::SHOP_MANAGED
                        : ItemOriginVO::USER_PERSONAL
                ),
                shopId: $shopId,                    // ★ INDIVIDUAL は null
                createdByUserId: $principal->userId(),
                name: $draft->name()->value(),
                price: $price,
                explain: $draft->explain(),
                condition: $draft->condition(),
                category: $draft->category(),
                itemImage: $itemImage,
                remain: new StockCount(1),
            );

            $item->markPublished(new \DateTimeImmutable('now'));

            $this->itemRepository->save($item);
            $itemId = $item->id();

            // rawText（純粋データのみ）
            $rawText = trim(implode(' ', array_filter([
                (string) $draft->name()?->value(),
                (string) ($draft->explain() ?? ''),
                (string) ($draft->brand()?->value() ?? ''),
                (string) ($draft->condition() ?? ''),
                (string) ($draft->color() ?? ''),
            ])));

            $draft->markPublished();
            $this->draftRepository->save($draft);
        });

        // transaction 完了後に dispatch
        Event::dispatch(new ItemImported(
            $itemId,
            $rawText,
            $tenantId,
            'publish',
            $input->draftId,
        ));
    }
}