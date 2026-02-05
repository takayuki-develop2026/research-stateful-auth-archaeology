<?php

namespace App\Modules\Payment\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\Dto\AdyenCommitInput;
use App\Modules\Payment\Application\UseCase\CommitAdyenCheckoutUseCase;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

final class AdyenCommitController extends Controller
{
    public function __construct(
        private CommitAdyenCheckoutUseCase $useCase
    ) {}

    /**
     * POST /api/payments/adyen/commit
     * {
     *   "preview_key": "uuid",
     *   "shop_id": 1,
     *   "items": [...],
     *   "address_id": 123
     * }
     */
    public function commit(Request $request): JsonResponse
    {
        $request->validate([
            'preview_key' => 'required|string',
            'shop_id' => 'required|integer',
            'items'   => 'required|array|min:1',
            'items.*.item_id' => 'required|integer',
            'items.*.name' => 'required|string',
            'items.*.price_amount' => 'required|integer|min:0',
            'items.*.price_currency' => 'required|string|max:10',
            'items.*.quantity' => 'nullable|integer|min:1',
            'items.*.image_path' => 'nullable|string',
            'address_id' => 'required|integer',
            'meta' => 'nullable|array',
        ]);

        $user = $request->user();
        if (! $user) return response()->json(['message' => 'Unauthenticated'], 401);

        try {
            $out = $this->useCase->handle(new AdyenCommitInput(
                userId: (int) $user->id,
                previewKey: (string) $request->input('preview_key'),
                shopId: (int) $request->input('shop_id'),
                items: $request->input('items'),
                addressId: (int) $request->input('address_id'),
                meta: $request->input('meta'),
            ));

            return response()->json($out, 200);
        } catch (\DomainException $e) {
            return response()->json(['message' => $e->getMessage()], 422);
        } catch (\Throwable $e) {
            \Log::error('[AdyenCommit failed]', ['exception' => $e]);
            return response()->json(['message' => 'commit failed'], 500);
        }
    }
}