<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Wallet;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\UseCase\Wallet\CompleteSetupIntentUseCase;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

final class CompleteSetupIntentController extends Controller
{
    public function __construct(
        private CompleteSetupIntentUseCase $useCase,
    ) {
    }

    /**
     * POST /api/wallet/setup-intent/complete
     * body: { setup_intent_id: "seti_xxx" }
     */
    public function __invoke(Request $request): JsonResponse
    {
        $user = $request->user();
        if (! $user) {
            return response()->json(['message' => 'Unauthenticated'], 401);
        }

        $setupIntentId = (string) $request->input('setup_intent_id', '');
        if ($setupIntentId === '') {
            return response()->json(['message' => 'setup_intent_id is required'], 422);
        }

        $out = $this->useCase->handle(
            userId: (int) $user->id,
            setupIntentId: $setupIntentId,
            provider: 'stripe',
            makeDefault: true,
        );

        return response()->json($out->toArray(), 200);
    }
}