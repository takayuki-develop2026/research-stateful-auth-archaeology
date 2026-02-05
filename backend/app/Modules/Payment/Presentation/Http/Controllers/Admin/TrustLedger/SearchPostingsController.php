<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\UseCase\Admin\TrustLedger\SearchPostingsUseCase;
use Illuminate\Http\Request;

final class SearchPostingsController extends Controller
{
    public function __construct(
        private SearchPostingsUseCase $useCase,
    ) {
    }

    public function __invoke(Request $request)
    {
        $data = $request->validate([
            'from' => 'required|date_format:Y-m-d',
            'to' => 'required|date_format:Y-m-d',
            'currency' => 'sometimes|string|in:JPY',
            'shop_ids' => 'sometimes|string', // "1,2,3"
            'posting_type' => 'sometimes|string|in:all,sale,fee,refund',

            // ✅ providerフィルタはこれだけに統一（ledger_postings.source_provider）
            'source_provider' => 'sometimes|string|in:all,stripe,adyen,paypal,unknown',

            'q' => 'sometimes|string',
            'limit' => 'sometimes|integer|min:1|max:200',
            'cursor' => 'sometimes|string',

            'payment_id' => 'sometimes|integer',
            'order_id' => 'sometimes|integer',
            'source_event_id' => 'sometimes|string',
        ]);

        $shopIds = null;
        if (!empty($data['shop_ids'])) {
            $shopIds = array_values(array_filter(array_map(
                fn ($v) => is_numeric($v) ? (int)$v : null,
                explode(',', $data['shop_ids'])
            )));
        }

        $page = $this->useCase->handle(
            shopIds: $shopIds,
            from: $data['from'],
            to: $data['to'],
            currency: $data['currency'] ?? 'JPY',
            postingType: $data['posting_type'] ?? 'all',
            sourceProvider: $data['source_provider'] ?? 'all',
            q: $data['q'] ?? null,
            paymentId: $data['payment_id'] ?? null,
            orderId: $data['order_id'] ?? null,
            sourceEventId: $data['source_event_id'] ?? null,
            limit: (int)($data['limit'] ?? 50),
            cursor: $data['cursor'] ?? null,
        );

        return response()->json($page->toArray(), 200);
    }
}
