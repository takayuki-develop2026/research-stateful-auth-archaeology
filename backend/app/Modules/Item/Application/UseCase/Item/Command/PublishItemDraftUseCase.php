<?php

declare(strict_types=1);

namespace App\Modules\Item\Application\UseCase\Item\Command;

use DateTimeImmutable;
use App\Modules\Auth\Application\Context\AuthContext;
use App\Modules\Item\Application\Dto\Item\PublishItemDraftInput;
use App\Modules\Item\Domain\Entity\Item;
use App\Modules\Item\Domain\Repository\ItemDraftRepository;
use App\Modules\Item\Domain\Repository\ItemRepository;
use App\Modules\Item\Domain\ValueObject\{
    ItemOrigin,
    CategoryList
};

final class PublishItemDraftUseCase
{
    public function __construct(
        private ItemDraftRepository $drafts,
        private ItemRepository $items,
    ) {}

public function handle(PublishItemDraftInput $input, AuthContext $auth): void
{
    $principal = $auth->principalOrNull();
    if (!$principal) throw new \RuntimeException('Unauthenticated');

    $draft = $this->drafts->findById($input->draftId);
    if (!$draft) throw new \RuntimeException('Draft not found');

    // publish入力を最終SoTに固定
    $draft->applyPublishIdentity(
        itemOrigin: $input->itemOrigin, // ← ここは lower が来る前提
        shopId: $input->shopId,
        userId: $principal->userId(),
    );
\Log::info('[🔥PublishDraft][AUTH CHECK]', [
  'principal_user_id' => $principal->userId(),
  'principal_shop_roles' => $principal->shopRoles(),
  'principal_shop_ids' => $principal->shopIds(),
  'input_origin' => $input->itemOrigin,
  'input_shop_id' => $input->shopId,
  'draft_shop_id' => $draft->shopId(),
]);
    // SHOP_MANAGED のとき：そのshopを持つか最低限チェック
    if ($input->itemOrigin === ItemOrigin::SHOP_MANAGED) {
    if (!in_array((int) $draft->shopId(), $principal->shopIds(), true)) {
        throw new \DomainException('You cannot publish for this shop');
    }
}

    if (!$draft->isPublishableV1()) {
        throw new \DomainException('Draft is not publishable (need image & DRAFT status)');
    }

    $draft->markPublished();
    $this->drafts->save($draft);

    // ✅ Domain origin は lower なのでこのままでOK
    $origin = ItemOrigin::from($input->itemOrigin);

    $item = Item::createNew(
        itemOrigin: $origin,
        shopId: $draft->shopId(),
        createdByUserId: $origin->isUserPersonal() ? $principal->userId() : null,
        name: $draft->name()->value(),
        price: $draft->price(),
        explain: $draft->explain(),
        condition: $draft->condition(),
        category: new CategoryList($draft->category()->toArray()),
        itemImage: $draft->itemImage(),
        remain: $draft->remain(),
    );
\Log::info('[🔥PublishDraft] after applyPublishIdentity', [
  'input_origin' => $input->itemOrigin,
  'input_shop_id'=> $input->shopId,
  'draft_seller' => $draft->sellerId()->raw(),
  'draft_shop_id'=> $draft->shopId(),
]);
    $item->markPublished(new DateTimeImmutable('now'));
    $this->items->save($item);
}
}