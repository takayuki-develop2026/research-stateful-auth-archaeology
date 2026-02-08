<?php

namespace App\Modules\Item\Application\UseCase\Item\Command;

use App\Modules\Item\Application\Dto\Item\CreateItemDraftInput;
use App\Modules\Item\Application\Dto\Item\CreateItemDraftOutput;
use App\Modules\Item\Domain\Entity\ItemDraft;
use App\Modules\Item\Domain\Repository\ItemDraftRepository;
use App\Modules\Item\Domain\ValueObject\{
    ItemName,
    Money,
    BrandName,
    ItemStatus,
    SellerId,
    SellerType
};
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use App\Modules\Auth\Application\Service\AssignSellerRoleService;

final class CreateItemDraftUseCase
{
    public function __construct(
        private ItemDraftRepository $draftRepository,
        private AssignSellerRoleService $assignSellerRoleService,
    ) {
    }

    public function execute(
        CreateItemDraftInput $input,
        AuthPrincipal $principal,
    ): CreateItemDraftOutput {

        // ✅ 入力 seller_id を Domain 化（SoT）
        $sellerId = SellerId::fromRaw($input->sellerId);

        /* =========================================
         * SHOP 出品の場合のみ shop_id を確定
         * ========================================= */
        $shopId = null;

if ($sellerId->type() === SellerType::SHOP) {
    $shopId = $sellerId->id(); // shop:2 など

    if (! $shopId) {
        // ✅ shop:managed を許さない（設計固定）
        throw new \DomainException('seller_id must be shop:{id} for SHOP listing');
    }
}

if ($sellerId->type() === SellerType::INDIVIDUAL) {
    $this->assignSellerRoleService->assignIndividualIfNotExists($principal->userId());
}

        /* =========================================
         * 個人出品のみ seller ロール付与
         * ========================================= */
        if ($sellerId->type() === SellerType::INDIVIDUAL) {
            $this->assignSellerRoleService
                ->assignIndividualIfNotExists($principal->userId());
        }

        /* =========================================
         * Draft 作成
         * ========================================= */
        $draftId = $this->draftRepository->nextIdentity();

        $draft = ItemDraft::create(
            id: $draftId,
            sellerId: $sellerId,
            shopId: $shopId,
            name: new ItemName($input->name),
            price: new Money($input->priceAmount, $input->priceCurrency),
            brandRaw: $input->brandRaw
                ? new BrandName($input->brandRaw)
                : null,
            explain: $input->explain,
            condition: $input->condition,
            category: $input->category,
        );

        $this->draftRepository->save($draft);

        return new CreateItemDraftOutput(
            $draftId->value(),
            ItemStatus::DRAFT->value,
            true
        );
    }
}
