<?php

namespace App\Modules\Item\Presentation\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;
use App\Http\Controllers\Controller;
use App\Modules\Item\Application\UseCase\Item\Command\PublishItemUseCase;
use App\Modules\Item\Application\Dto\Item\PublishItemInput;
use App\Modules\Auth\Application\Context\AuthContext;

final class PublishItemController extends Controller
{
    public function __construct(
        private PublishItemUseCase $useCase,
        private AuthContext $authContext,
    ) {
    }

    public function __invoke(Request $request, string $draftId): JsonResponse
{
    $principal = $this->authContext->principal();
    if (! $principal) {
        return response()->json(['message' => 'Unauthenticated'], 401);
    }

    $shopIdRaw = $request->input('shop_id');

    $input = new PublishItemInput(
        draftId: $draftId,
        shopId: is_null($shopIdRaw) ? null : (int) $shopIdRaw,
    );

    $this->useCase->execute($input, $principal, null);

    return response()->json(['status' => 'ok']);
}
}