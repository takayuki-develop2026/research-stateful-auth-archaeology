<?php

namespace App\Modules\Reaction\Application\UseCase\Query;

use App\Modules\Reaction\Domain\Repository\FavoriteRepository;
use App\Modules\Item\Application\Assembler\PublicItemAssembler;
use App\Modules\Item\Application\Port\FavoriteItemReadPort;
use App\Modules\Reaction\Domain\ValueObject\FavoriteTargetId;

final class ListFavoriteUseCase
{
    public function __construct(
        private FavoriteRepository $favoriteRepository,
        private FavoriteItemReadPort $favoriteItems,
    ) {
    }

    /**
     * @return array<int, array<string,mixed>> 返却は API DTO(toArray) を固定
     */
    public function execute(int $viewerUserId): array
    {
        $rows = $this->favoriteItems->listByUserId($viewerUserId);

        return collect($rows)
            ->map(function (array $row) use ($viewerUserId) {

                $itemId = (int) ($row['id'] ?? 0);

                $favoritesCount = $this->favoriteRepository->countByTarget(
                    new FavoriteTargetId($itemId)
                );

                $dto = PublicItemAssembler::fromReadModel(
                    row: $row,
                    viewerUserId: $viewerUserId,
                    viewerShopIds: [],
                    isFavorited: true,
                    favoritesCount: $favoritesCount,
                );

                return $dto->toArray(); // ✅ ここが核心
            })
            ->values()
            ->all();
    }
}