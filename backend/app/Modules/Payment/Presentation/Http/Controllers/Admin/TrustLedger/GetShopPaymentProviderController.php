<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use Illuminate\Support\Facades\DB;

final class GetShopPaymentProviderController extends Controller
{
    public function __invoke(int $shopId)
    {
        $row = DB::table('shops')
            ->where('id', $shopId)
            ->select(['id', 'name', 'shop_code', 'payment_provider', 'payment_provider_meta', 'payment_provider_updated_at'])
            ->first();

        if (! $row) {
            return response()->json(['message' => 'Shop not found'], 404);
        }

        return response()->json([
            'shop_id' => (int)$row->id,
            'shop_name' => $row->name,
            'shop_code' => $row->shop_code,
            'payment_provider' => $row->payment_provider ?? 'stripe',
            'payment_provider_meta' => $row->payment_provider_meta,
            'payment_provider_updated_at' => $row->payment_provider_updated_at,
        ], 200);
    }
}