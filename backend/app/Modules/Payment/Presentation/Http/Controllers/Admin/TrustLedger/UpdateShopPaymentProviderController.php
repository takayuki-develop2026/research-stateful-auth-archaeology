<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\UseCase\Admin\TrustLedger\UpdateShopPaymentProviderUseCase;
use Illuminate\Http\Request;

final class UpdateShopPaymentProviderController extends Controller
{
    public function __construct(
        private UpdateShopPaymentProviderUseCase $useCase,
    ) {}

    public function __invoke(Request $request)
    {
        $data = $request->validate([
            'mode' => 'required|string|in:row,bulk',
            'provider' => 'required|string|in:stripe,adyen',

            'shop_id' => 'sometimes|integer|min:1',
            'shop_ids' => 'sometimes|array',
            'shop_ids.*' => 'integer|min:1',
        ]);

        if ($data['mode'] === 'row') {
            if (empty($data['shop_id'])) {
                return response()->json(['message' => 'shop_id required'], 422);
            }
            $res = $this->useCase->handleRow((int)$data['shop_id'], $data['provider']);
            return response()->json(['ok' => true, 'result' => $res], 200);
        }

        // bulk
        $shopIds = $data['shop_ids'] ?? null;
        if (!is_array($shopIds)) {
            return response()->json(['message' => 'shop_ids required'], 422);
        }
        $shopIds = array_map(fn ($v) => (int)$v, $shopIds);

        $res = $this->useCase->handleBulk($shopIds, $data['provider']);
        return response()->json(['ok' => true, 'result' => $res], 200);
    }
}