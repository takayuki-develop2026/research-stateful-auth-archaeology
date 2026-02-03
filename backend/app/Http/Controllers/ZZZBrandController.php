<?php

namespace App\Http\Controllers;

use App\Jobs\AnalyzeEntityWithAtlasKernel;
use Illuminate\Support\Facades\Http;

class BrandController extends Controller
{
    public function store(Request $request)
    {
        $res = Http::post('http://python_atlaskernel:8000/analyze', [
            'entity_type' => 'brand',
            'raw_value' => $request->input('brand_name'),
            'known_assets_ref' => 'brands_v1',
        ]);

        $payload = $res->json();

        return response()->json([
            'canonical_brand' => $payload['canonical_value'],
            'confidence' => $payload['confidence'],
        ]);
    }
}
