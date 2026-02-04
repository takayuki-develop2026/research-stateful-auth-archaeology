<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Domain\Enum\PaymentProvider;
use App\Modules\Payment\Domain\Shop\Repository\AdminShopQueryRepository;
use Illuminate\Http\Request;
use Illuminate\Validation\Rule;

final class ListShopsController extends Controller
{
    public function __construct(
        private AdminShopQueryRepository $shops,
    ) {}

    public function __invoke(Request $request)
    {
        $data = $request->validate([
            'q'        => ['nullable','string','max:200'],
            'status'   => ['nullable','string', Rule::in(['active','inactive'])],
            'provider' => ['nullable','string', Rule::in(array_map(fn($c) => $c->value, PaymentProvider::cases()))],
            'limit'    => ['nullable','integer','min:1','max:200'],
            'page'     => ['nullable','integer','min:1','max:100000'],
        ]);

        // 空文字を null 化（nullableでも、クエリに ?status= が来ると "" になるケースがある）
        $q        = $this->blankToNull($data['q'] ?? null);
        $status   = $this->blankToNull($data['status'] ?? null);
        $provider = $this->blankToNull($data['provider'] ?? null);

        $limit = (int)($data['limit'] ?? 50);
        $page  = (int)($data['page'] ?? 1);

        $result = $this->shops->search(
            q: $q,
            status: $status,
            provider: $provider,
            limit: $limit,
            page: $page,
        );

        return response()->json($result, 200);
    }

    private function blankToNull(?string $v): ?string
    {
        if ($v === null) return null;
        $v = trim($v);
        return $v === '' ? null : $v;
    }
}