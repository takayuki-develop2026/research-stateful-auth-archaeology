<?php

namespace App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger;

use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerKpiQueryRepository;
use Illuminate\Support\Facades\DB;

final class EloquentAdminLedgerKpiQueryRepository implements AdminLedgerKpiQueryRepository
{
    public function getGlobalKpi(string $from, string $to, string $currency): array
    {
        $fromAt = $from . ' 00:00:00';
        $toAt   = $to . ' 23:59:59';

        // ---- totals（既存互換）----
        $rows = DB::table('ledger_postings')
            ->select(
                'posting_type',
                DB::raw('SUM(amount) as s'),
                DB::raw('COUNT(*) as c')
            )
            ->where('currency', $currency)
            ->whereBetween('occurred_at', [$fromAt, $toAt])
            ->whereIn('posting_type', ['sale', 'refund', 'fee'])
            ->groupBy('posting_type')
            ->get();

        $sales = 0; $refund = 0; $fee = 0; $count = 0;
        foreach ($rows as $r) {
            $count += (int)$r->c;
            if ($r->posting_type === 'sale')   $sales  = (int)$r->s;
            if ($r->posting_type === 'refund') $refund = (int)$r->s;
            if ($r->posting_type === 'fee')    $fee    = (int)$r->s;
        }

        // ---- by_provider（追加）----
        $rows2 = DB::table('ledger_postings')
            ->select(
                DB::raw("COALESCE(NULLIF(source_provider,''),'unknown') as provider"),
                'posting_type',
                DB::raw('SUM(amount) as s'),
                DB::raw('COUNT(*) as c')
            )
            ->where('currency', $currency)
            ->whereBetween('occurred_at', [$fromAt, $toAt])
            ->whereIn('posting_type', ['sale', 'refund', 'fee'])
            ->groupBy('provider', 'posting_type')
            ->get();

        /**
         * by_provider = [
         *   'stripe' => ['sales'=>int,'refund'=>int,'fee'=>int,'postings_count'=>int],
         *   'adyen'  => ...
         * ]
         */
        $byProvider = [];
        foreach ($rows2 as $r) {
            $p = (string)$r->provider;
            if (!isset($byProvider[$p])) {
                $byProvider[$p] = [
                    'sales' => 0,
                    'refund' => 0,
                    'fee' => 0,
                    'postings_count' => 0,
                ];
            }

            $byProvider[$p]['postings_count'] += (int)$r->c;

            if ($r->posting_type === 'sale')   $byProvider[$p]['sales']  = (int)$r->s;
            if ($r->posting_type === 'refund') $byProvider[$p]['refund'] = (int)$r->s;
            if ($r->posting_type === 'fee')    $byProvider[$p]['fee']    = (int)$r->s;
        }

        return [
            'sales' => $sales,
            'refund' => $refund,
            'fee' => $fee,
            'postings_count' => $count,
            'by_provider' => $byProvider,
        ];
    }

    public function getShopKpis(?array $shopIds, string $from, string $to, string $currency): array
    {
        $fromAt = $from . ' 00:00:00';
        $toAt   = $to . ' 23:59:59';

        // ---- totals per shop（既存互換）----
        $q = DB::table('ledger_postings')
            ->select(
                'shop_id',
                'posting_type',
                DB::raw('SUM(amount) as s'),
                DB::raw('COUNT(*) as c')
            )
            ->where('currency', $currency)
            ->whereBetween('occurred_at', [$fromAt, $toAt])
            ->whereIn('posting_type', ['sale', 'refund', 'fee'])
            ->groupBy('shop_id', 'posting_type');

        if (is_array($shopIds) && count($shopIds) > 0) {
            $q->whereIn('shop_id', $shopIds);
        }

        $rows = $q->get();

        /**
         * map[shop_id] = [
         *   shop_id, sales, refund, fee, postings_count,
         *   by_provider => [ provider => {sales/refund/fee/postings_count} ]
         * ]
         */
        $map = [];
        foreach ($rows as $r) {
            $sid = (int)$r->shop_id;
            if (!isset($map[$sid])) {
                $map[$sid] = [
                    'shop_id' => $sid,
                    'sales' => 0,
                    'refund' => 0,
                    'fee' => 0,
                    'postings_count' => 0,
                    'by_provider' => [],
                ];
            }
            $map[$sid]['postings_count'] += (int)$r->c;

            if ($r->posting_type === 'sale')   $map[$sid]['sales']  = (int)$r->s;
            if ($r->posting_type === 'refund') $map[$sid]['refund'] = (int)$r->s;
            if ($r->posting_type === 'fee')    $map[$sid]['fee']    = (int)$r->s;
        }

        // ---- by_provider per shop（追加）----
        $q2 = DB::table('ledger_postings')
            ->select(
                'shop_id',
                DB::raw("COALESCE(NULLIF(source_provider,''),'unknown') as provider"),
                'posting_type',
                DB::raw('SUM(amount) as s'),
                DB::raw('COUNT(*) as c')
            )
            ->where('currency', $currency)
            ->whereBetween('occurred_at', [$fromAt, $toAt])
            ->whereIn('posting_type', ['sale', 'refund', 'fee'])
            ->groupBy('shop_id', 'provider', 'posting_type');

        if (is_array($shopIds) && count($shopIds) > 0) {
            $q2->whereIn('shop_id', $shopIds);
        }

        $rows2 = $q2->get();

        foreach ($rows2 as $r) {
            $sid = (int)$r->shop_id;
            if (!isset($map[$sid])) {
                // 念のため（基本ここには来ない）
                $map[$sid] = [
                    'shop_id' => $sid,
                    'sales' => 0,
                    'refund' => 0,
                    'fee' => 0,
                    'postings_count' => 0,
                    'by_provider' => [],
                ];
            }

            $p = (string)$r->provider;
            if (!isset($map[$sid]['by_provider'][$p])) {
                $map[$sid]['by_provider'][$p] = [
                    'sales' => 0,
                    'refund' => 0,
                    'fee' => 0,
                    'postings_count' => 0,
                ];
            }

            $map[$sid]['by_provider'][$p]['postings_count'] += (int)$r->c;

            if ($r->posting_type === 'sale')   $map[$sid]['by_provider'][$p]['sales']  = (int)$r->s;
            if ($r->posting_type === 'refund') $map[$sid]['by_provider'][$p]['refund'] = (int)$r->s;
            if ($r->posting_type === 'fee')    $map[$sid]['by_provider'][$p]['fee']    = (int)$r->s;
        }

        return $map;
    }
}