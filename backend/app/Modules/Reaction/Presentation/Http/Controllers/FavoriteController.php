<?php

namespace App\Modules\Reaction\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use Illuminate\Http\Request;
use App\Modules\Reaction\Application\UseCase\Command\AddFavoriteUseCase;
use App\Modules\Reaction\Application\UseCase\Command\RemoveFavoriteUseCase;
use App\Modules\Reaction\Application\UseCase\Query\ListFavoriteUseCase;
use App\Modules\Reaction\Application\UseCase\Query\CountFavoritesUseCase;

final class FavoriteController extends Controller
{
    public function __construct(
        private readonly ListFavoriteUseCase $listFavorites,
    ) {
    }

    /**
     * GET /api/favorites
     * middleware: auth.occ
     */
    public function index(Request $request)
    {
        $user = $request->user(); // auth.occ 前提なら null にならない
        $items = $this->listFavorites->execute((int) $user->id);

        // ✅ DTO配列 → array に正規化（v3固定）
        $payload = array_map(
            fn ($dto) => is_object($dto) && method_exists($dto, 'toArray') ? $dto->toArray() : $dto,
            $items
        );

        return response()->json([
            'items' => $payload,
        ], 200);
    }

    /**
     * POST /api/favorites/{itemId}
     * middleware: auth.occ
     */
    public function add(
        AddFavoriteUseCase $add,
        CountFavoritesUseCase $count,
        Request $request,
        int $itemId
    ) {
        $userId = (int) $request->user()->id;

        $add->execute($userId, $itemId);

        return response()->json([
            'favorited' => true,
            'favorites_count' => $count->execute($itemId),
        ], 200);
    }

    /**
     * DELETE /api/favorites/{itemId}
     * middleware: auth.occ
     */
    public function remove(
        RemoveFavoriteUseCase $remove,
        CountFavoritesUseCase $count,
        Request $request,
        int $itemId
    ) {
        $userId = (int) $request->user()->id;

        $remove->execute($userId, $itemId);

        return response()->json([
            'favorited' => false,
            'favorites_count' => $count->execute($itemId),
        ], 200);
    }
}