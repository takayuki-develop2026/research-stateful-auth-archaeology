<?php

declare(strict_types=1);

namespace App\Modules\Item\Infrastructure\Persistence\Repository;

use Illuminate\Support\Facades\DB;
use Illuminate\Database\Eloquent\ModelNotFoundException;
use App\Modules\Item\Domain\Repository\AnalysisRequestRepository;
use App\Modules\Item\Domain\ValueObject\AnalysisRequestRecord;

final class EloquentAnalysisRequestRepository implements AnalysisRequestRepository
{
    /**
     * Atlas 管理画面用（shop scope / raw rows）
     * @return array<int, \stdClass>
     */
    public function listByShopCode(string $shopCode): array
    {
        return $this->baseListQuery()
            ->where('s.shop_code', $shopCode)
            ->orderByDesc('ar.created_at')
            ->get()
            ->all();
    }

    /**
     * Atlas 管理画面用（admin scope / 全shop / raw rows）
     * @return array<int, \stdClass>
     */
    public function listAllShops(): array
    {
        return $this->baseListQuery()
            ->orderByDesc('ar.created_at')
            ->get()
            ->all();
    }

    /**
     * 一覧共通クエリ
     */
    private function baseListQuery()
    {
        return DB::table('analysis_requests as ar')
            // shop の正しい辿り方
            ->leftJoin('items as i', 'i.id', '=', 'ar.item_id')
            ->leftJoin('shops as s', 's.id', '=', 'i.shop_id')

            // 解析前（Human Input）
            ->leftJoin('item_drafts as d', 'd.id', '=', 'ar.item_draft_id')

            // AI result
            ->leftJoin('analysis_results as res', function ($join) {
                $join->on('res.analysis_request_id', '=', 'ar.id')
                    ->whereIn('res.status', ['active', 'provisional']);
            })

            // latest review decision
            ->leftJoin('review_decisions as rd', function ($join) {
                $join->on('rd.analysis_request_id', '=', 'ar.id')
                    ->whereRaw('rd.id = (
                        select rd2.id
                        from review_decisions rd2
                        where rd2.analysis_request_id = ar.id
                        order by rd2.decided_at desc, rd2.id desc
                        limit 1
                     )');
            })

            ->leftJoin('users as u', 'u.id', '=', 'rd.decided_by')

            // Final SoT
            ->leftJoin('item_entities as ie', function ($join) {
                $join->on('ie.item_id', '=', 'ar.item_id')
                    ->where('ie.is_latest', true);
            })
            ->leftJoin('brand_entities as be', 'be.id', '=', 'ie.brand_entity_id')
            ->leftJoin('condition_entities as ce', 'ce.id', '=', 'ie.condition_entity_id')
            ->leftJoin('color_entities as coe', 'coe.id', '=', 'ie.color_entity_id')

            // SEED ダミー除外は共通
            ->where('i.name', 'not like', 'SEED\_DUMMY\_\_%')

            ->select([
                // ===== Core =====
                'ar.id as request_id',
                's.shop_code',
                'ar.analysis_version',
                'ar.status as request_status',
                'ar.created_at as analyzed_at',

                // ===== Trigger =====
                'ar.triggered_by_type',
                'ar.trigger_reason',
                'ar.original_request_id',
                'ar.replay_index',

                // ===== Error =====
                'ar.error_code',
                'ar.error_message',

                // ===== Item =====
                'i.id as item_id',
                'i.name as item_name',

                // ===== Before (Human Input) =====
                'd.brand as before_brand',
                'd.condition as before_condition',
                DB::raw('NULL as before_color'),
                'd.created_at as submitted_at',

                // ===== AI =====
                'res.brand_name as ai_brand',
                'res.condition_name as ai_condition',
                'res.color_name as ai_color',
                'res.overall_confidence as max_confidence',
                'res.source as ai_source',
                'res.confidence_map',

                // ===== Decision =====
                'rd.decision_type',
                'rd.decided_by_type',
                'rd.decided_at',
                'u.id as user_id',
                'u.name as user_name',

                // ===== Diff =====
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.before_snapshot, '$.brand.value')) as diff_brand_before"),
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.after_snapshot, '$.brand.value')) as diff_brand_after"),
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.before_snapshot, '$.condition.value')) as diff_condition_before"),
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.after_snapshot, '$.condition.value')) as diff_condition_after"),
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.before_snapshot, '$.color.value')) as diff_color_before"),
                DB::raw("JSON_UNQUOTE(JSON_EXTRACT(rd.after_snapshot, '$.color.value')) as diff_color_after"),

                // ===== Final =====
                'be.canonical_name as final_brand',
                'ie.source as final_source',
                'ce.canonical_name as final_condition',
                'coe.canonical_name as final_color',

                // ===== Final Confidence (from latest decision after_snapshot) =====
                DB::raw("CAST(JSON_EXTRACT(rd.after_snapshot, '$.brand.confidence') AS DECIMAL(10,4)) as final_brand_confidence"),
                DB::raw("CAST(JSON_EXTRACT(rd.after_snapshot, '$.condition.confidence') AS DECIMAL(10,4)) as final_condition_confidence"),
                DB::raw("CAST(JSON_EXTRACT(rd.after_snapshot, '$.color.confidence') AS DECIMAL(10,4)) as final_color_confidence"),
                DB::raw("
                    GREATEST(
                        COALESCE(CAST(JSON_EXTRACT(rd.after_snapshot, '$.brand.confidence') AS DECIMAL(10,4)), 0),
                        COALESCE(CAST(JSON_EXTRACT(rd.after_snapshot, '$.condition.confidence') AS DECIMAL(10,4)), 0),
                        COALESCE(CAST(JSON_EXTRACT(rd.after_snapshot, '$.color.confidence') AS DECIMAL(10,4)), 0)
                    ) as final_max_confidence
                "),
            ]);
    }

    /* ===== interface 実装 ===== */

    public function create(array $attributes): int
    {
        return DB::table('analysis_requests')->insertGetId([
            'tenant_id'        => $attributes['tenant_id'] ?? null,
            'item_id'          => $attributes['item_id'],
            'item_draft_id'    => $attributes['item_draft_id'] ?? null,
            'analysis_version' => $attributes['analysis_version'],
            'raw_text'         => $attributes['raw_text'],
            'payload_hash'     => $attributes['payload_hash'],
            'idempotency_key'  => $attributes['idempotency_key'],
            'status'           => 'pending',
            'created_at'       => now(),
            'updated_at'       => now(),
        ]);
    }

    public function findOrFail(int $requestId): AnalysisRequestRecord
    {
        $row = DB::table('analysis_requests')->where('id', $requestId)->first();

        if (! $row) {
            throw new ModelNotFoundException();
        }

        return new AnalysisRequestRecord(
            id: (int) $row->id,
            tenantId: $row->tenant_id,
            itemId: (int) $row->item_id,
            itemDraftId: $row->item_draft_id,
            analysisVersion: $row->analysis_version,
            rawText: $row->raw_text,
            status: $row->status,
        );
    }

    public function markDone(int $requestId): void
    {
        DB::table('analysis_requests')
            ->where('id', $requestId)
            ->update([
                'status' => 'done',
                'updated_at' => now(),
            ]);
    }

    public function markFailed(
        int $requestId,
        ?string $errorCode = null,
        ?string $errorMessage = null,
    ): void {
        DB::table('analysis_requests')
            ->where('id', $requestId)
            ->update([
                'status' => 'failed',
                'error_code' => $errorCode,
                'error_message' => $errorMessage,
                'updated_at' => now(),
            ]);
    }
}