<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger;

use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerPostingQueryRepository;
use Illuminate\Support\Facades\DB;

final class EloquentAdminLedgerPostingQueryRepository implements AdminLedgerPostingQueryRepository
{
    public function searchPostings(
        ?array $shopIds,
        string $from,
        string $to,
        string $currency,
        string $postingType,
        string $sourceProvider, // ✅ providerはここで固定（all可）
        ?string $q,
        ?int $paymentId,
        ?int $orderId,
        ?string $sourceEventId,
        int $limit,
        ?string $cursor
    ): array {
        $limit = max(1, min($limit, 200));

        $qb = DB::table('ledger_postings')
            ->select([
                'id',
                'shop_id',
                'occurred_at',
                'posting_type',
                'order_id',
                'payment_id',
                'source_provider',
                'source_event_id',
                'amount',
                'currency',
            ])
            ->where('currency', $currency)
            ->whereBetween('occurred_at', [$from . ' 00:00:00', $to . ' 23:59:59']);

        if (is_array($shopIds) && count($shopIds) > 0) {
            $qb->whereIn('shop_id', $shopIds);
        }

        if ($postingType !== 'all') {
            $qb->where('posting_type', $postingType);
        }

        // ✅ source_provider フィルタ（all のときは何もしない）
        if ($sourceProvider !== 'all') {
            $qb->where('source_provider', $sourceProvider);
        }

        // ✅ 正式フィルタ
        if (is_int($paymentId)) {
            $qb->where('payment_id', $paymentId);
        }
        if (is_int($orderId)) {
            $qb->where('order_id', $orderId);
        }
        if (is_string($sourceEventId) && $sourceEventId !== '') {
            $qb->where('source_event_id', 'like', trim($sourceEventId) . '%');
        }

        // ✅ 自由検索（q）
        if (is_string($q) && $q !== '') {
            $qq = trim($q);
            if (ctype_digit($qq)) {
                $qb->where(function ($w) use ($qq) {
                    $w->where('order_id', (int)$qq)
                        ->orWhere('payment_id', (int)$qq)
                        ->orWhere('source_event_id', 'like', $qq . '%');
                });
            } else {
                $qb->where('source_event_id', 'like', $qq . '%');
            }
        }

        if (is_string($cursor) && ctype_digit($cursor)) {
            $qb->where('id', '<', (int)$cursor);
        }

        $rows = $qb->orderByDesc('id')->limit($limit + 1)->get();

        $items = [];
        foreach ($rows->take($limit) as $r) {
            $items[] = [
                'posting_id' => (int)$r->id,
                'shop_id' => (int)$r->shop_id,
                'occurred_at' => (string)$r->occurred_at,
                'posting_type' => (string)$r->posting_type,
                'order_id' => $r->order_id !== null ? (int)$r->order_id : null,
                'payment_id' => $r->payment_id !== null ? (int)$r->payment_id : null,
                'source_provider' => (string)$r->source_provider,
                'source_event_id' => (string)$r->source_event_id,
                'amount' => (int)$r->amount,
                'currency' => (string)$r->currency,
            ];
        }

        $next = null;
        if ($rows->count() > $limit) {
            $next = (string)$rows->last()->id;
        }

        return ['items' => $items, 'next_cursor' => $next];
    }

    public function getPostingDetail(int $postingId): array
    {
        $p = DB::table('ledger_postings')->where('id', $postingId)->first();
        if (!$p) {
            throw new \DomainException('Posting not found');
        }

        $entries = DB::table('ledger_entries')
            ->select(['account_code', 'side', 'amount', 'currency'])
            ->where('posting_id', $postingId)
            ->orderBy('id', 'asc')
            ->get()
            ->map(fn ($e) => [
                'account_code' => (string)$e->account_code,
                'side' => (string)$e->side,
                'amount' => (int)$e->amount,
                'currency' => (string)$e->currency,
            ])
            ->all();

        $posting = [
            'posting_id' => (int)$p->id,
            'shop_id' => (int)$p->shop_id,
            'occurred_at' => (string)$p->occurred_at,
            'posting_type' => (string)$p->posting_type,
            'order_id' => $p->order_id !== null ? (int)$p->order_id : null,
            'payment_id' => $p->payment_id !== null ? (int)$p->payment_id : null,
            'source_provider' => (string)$p->source_provider,
            'source_event_id' => (string)$p->source_event_id,
            'amount' => (int)$p->amount,
            'currency' => (string)$p->currency,
            'meta' => $p->meta ? json_decode((string)$p->meta, true) : null,
        ];

        return ['posting' => $posting, 'entries' => $entries];
    }
}