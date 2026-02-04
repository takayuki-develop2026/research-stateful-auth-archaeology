<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;

final class SetShopPaymentProviderController extends Controller
{
    public function __invoke(Request $request, int $shopId)
    {
        $data = $request->validate([
            'payment_provider' => 'required|string|in:stripe,adyen',
            'payment_provider_meta' => 'sometimes|array',
        ]);

        $exists = DB::table('shops')->where('id', $shopId)->exists();
        if (! $exists) {
            return response()->json(['message' => 'Shop not found'], 404);
        }

        DB::table('shops')
            ->where('id', $shopId)
            ->update([
                'payment_provider' => $data['payment_provider'],
                'payment_provider_meta' => array_key_exists('payment_provider_meta', $data)
                    ? json_encode($data['payment_provider_meta'])
                    : DB::raw('payment_provider_meta'),
                'payment_provider_updated_at' => now(),
                'updated_at' => now(),
            ]);

        return response()->json([
            'ok' => true,
            'shop_id' => $shopId,
            'payment_provider' => $data['payment_provider'],
        ], 200);
    }
}