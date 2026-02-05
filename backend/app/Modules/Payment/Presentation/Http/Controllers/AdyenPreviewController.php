<?php

namespace App\Modules\Payment\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\Dto\CreateAdyenPreviewSessionInput;
use App\Modules\Payment\Application\UseCase\CreateAdyenPreviewSessionUseCase;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

final class AdyenPreviewController extends Controller
{
    public function __construct(
        private CreateAdyenPreviewSessionUseCase $useCase
    ) {}

    public function __invoke(Request $request): JsonResponse
    {
        $request->validate([
            'shop_id' => 'required|integer',
            'amount' => 'required|integer|min:1',
            'currency' => 'nullable|string|max:10',
        ]);

        $user = $request->user();
        if (! $user) return response()->json(['message' => 'Unauthenticated'], 401);

        $out = $this->useCase->handle(new CreateAdyenPreviewSessionInput(
            userId: (int) $user->id,
            shopId: (int) $request->input('shop_id'),
            amount: (int) $request->input('amount'),
            currency: (string) ($request->input('currency') ?? 'JPY'),
        ));

        return response()->json($out->toArray(), 200);
    }
}