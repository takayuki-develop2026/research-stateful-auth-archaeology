<?php

namespace App\Modules\Order\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Order\Application\UseCase\ConfirmOrderAddressUseCase;
use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;

final class ConfirmOrderAddressController extends Controller
{
    public function __construct(
        private ConfirmOrderAddressUseCase $useCase
    ) {
    }

    public function __invoke(Request $request, int $orderId): JsonResponse
    {
        $request->validate([
            'address_id' => 'required|integer',
        ]);

        $user = $request->user();
        if (! $user) {
            return response()->json(['message' => 'Unauthenticated'], 401);
        }

        $this->useCase->handle(
            orderId: $orderId,
            userId: (int) $user->id,
            addressId: (int) $request->input('address_id')
        );

        return response()->json(['status' => 'ok'], 200);
    }
}
