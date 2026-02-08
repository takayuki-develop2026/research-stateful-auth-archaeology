<?php

namespace App\Modules\Item\Application\UseCase\Item\Command;

use App\Modules\Item\Domain\Repository\ItemDraftRepository;
use App\Modules\Item\Domain\Service\SellerAuthorizationService;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use App\Modules\Item\Domain\ValueObject\ItemImagePath;
use DomainException;

final class UploadItemDraftImageUseCase
{
    public function __construct(
        private ItemDraftRepository $draftRepository,
        private SellerAuthorizationService $authorization,
    ) {
    }

    public function execute(
        string $draftId,
        AuthPrincipal $principal,
        string $path,
    ): void {
        $draft = $this->draftRepository->findById($draftId);
\Log::info('[🔥UploadDraftImage][AUTH CHECK]', [
    'draft_id' => $draftId,
    'draft_seller_raw' => $draft->sellerId()->raw(),
    'principal_user_id' => $principal->userId(),
    'principal_shop_ids' => $principal->shopIds(),
    'principal_shop_roles' => $principal->shopRoles(),
]);
        if (! $draft) {
            throw new DomainException('Draft not found');
        }

        if (! $this->authorization->canOperate(
            $draft->sellerId(),
            $principal
        )) {
            throw new DomainException('Forbidden');
        }

        $draft->attachImage(
            ItemImagePath::fromRaw($path)
        );

        $this->draftRepository->save($draft);
    }
}
