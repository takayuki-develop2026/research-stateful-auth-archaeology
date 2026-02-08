<?php

namespace App\Modules\Item\Presentation\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;
use App\Http\Controllers\Controller;
use App\Modules\Item\Application\UseCase\Item\Command\PublishItemDraftUseCase;
use App\Modules\Item\Application\Dto\Item\PublishItemDraftInput;
use App\Modules\Auth\Application\Context\AuthContext;
use App\Modules\Item\Domain\ValueObject\ItemOrigin;

final class PublishItemDraftController extends Controller
{
    public function __construct(
        private PublishItemDraftUseCase $useCase,
        private AuthContext $authContext,
    ) {}

    public function __invoke(Request $request, string $draftId): JsonResponse
    {
        $validated = $request->validate([
            'item_origin' => ['required', 'string'],
            'shop_id'     => ['nullable', 'integer'],
        ]);

        // ✅ どっちで来ても吸収（"SHOP_MANAGED" / "shop_managed" 両対応）
        $raw = trim((string) $validated['item_origin']);
        $lower = strtolower($raw);

        $origin = match (true) {
            $lower === ItemOrigin::SHOP_MANAGED  => ItemOrigin::SHOP_MANAGED,   // "shop_managed"
            $lower === ItemOrigin::USER_PERSONAL => ItemOrigin::USER_PERSONAL, // "user_personal"
            $raw === 'SHOP_MANAGED'              => ItemOrigin::SHOP_MANAGED,
            $raw === 'USER_PERSONAL'             => ItemOrigin::USER_PERSONAL,
            default => throw new \DomainException("Invalid item_origin: {$raw}"),
        };

        // ✅ shop_managed の時だけ shop_id 必須
        $shopId = $validated['shop_id'] ?? null;
        if ($origin === ItemOrigin::SHOP_MANAGED && empty($shopId)) {
            throw new \DomainException('shop_id is required for shop_managed');
        }

        $input = new PublishItemDraftInput(
            draftId: $draftId,
            itemOrigin: $origin,    // ✅ ここで100% lower固定
            shopId: $shopId ? (int)$shopId : null,
        );

        $this->useCase->handle($input, $this->authContext);

        return response()->json(['status' => 'ok']);
    }
}