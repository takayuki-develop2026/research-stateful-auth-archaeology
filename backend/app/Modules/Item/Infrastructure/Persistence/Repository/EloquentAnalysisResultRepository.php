<?php

namespace App\Modules\Item\Infrastructure\Persistence\Repository;

use App\Modules\Item\Domain\Repository\AnalysisResultRepository;
use App\Modules\Item\Domain\Entity\AnalysisResult;
use Illuminate\Support\Facades\DB;
use InvalidArgumentException;

final class EloquentAnalysisResultRepository implements AnalysisResultRepository
{
    /**
     * ✅ v3固定：requestId 主語で保存
     */
    public function saveByRequestId(int $requestId, array $payload): void
{
    if (isset($payload['analysis_request_id']) && (int)$payload['analysis_request_id'] !== $requestId) {
        throw new InvalidArgumentException('analysis_request_id mismatch');
    }

    $itemId = isset($payload['item_id']) && is_numeric($payload['item_id'])
        ? (int)$payload['item_id']
        : null;

    $data = [
        'item_id'            => $itemId,

        'brand_name'         => $payload['brand_name'] ?? null,
        'condition_name'     => $payload['condition_name'] ?? null,
        'color_name'         => $payload['color_name'] ?? null,

        'item_draft_id'  => $payload['item_draft_id'] ?? null,

        'classified_tokens'  => $this->toJsonOrNull($payload['classified_tokens'] ?? null),
        'confidence_map'     => $this->toJsonOrNull($payload['confidence_map'] ?? null),
        'overall_confidence' => is_numeric($payload['overall_confidence'] ?? null)
            ? (float)$payload['overall_confidence']
            : null,

        'evidence'           => $this->toJsonOrNull($payload['evidence'] ?? null),
        'source'             => $payload['source'] ?? null,
        'status'             => $payload['status'] ?? 'active',

        'updated_at'         => now(),
    ];

    $exists = DB::table('analysis_results')
        ->where('analysis_request_id', $requestId)
        ->exists();

    if ($exists) {
        DB::table('analysis_results')
            ->where('analysis_request_id', $requestId)
            ->update($data);
        return;
    }

    DB::table('analysis_results')->insert(array_merge($data, [
        'analysis_request_id' => $requestId,
        'created_at'          => now(),
    ]));
}

    /**
     * （任意）過去互換が必要なら残す。不要なら削除でOK。
     * ただし「主語は requestId」であることを崩さない。
     */
    // public function save(int $itemId, array $payload): void
    // {
    //     $requestId = $payload['analysis_request_id']
    //         ?? throw new InvalidArgumentException('analysis_request_id is required');

    //     $payload['item_id'] = $payload['item_id'] ?? $itemId;

    //     $this->saveByRequestId((int)$requestId, $payload);
    // }

    private function toJsonOrNull(mixed $value): ?string
    {
        if ($value === null) {
            return null;
        }

        if (is_string($value)) {
            // すでにJSONかもしれないが、ここはそのまま保存する方が安全
            return $value;
        }

        if (is_array($value)) {
            return json_encode($value, JSON_UNESCAPED_UNICODE);
        }

        if (is_object($value) && method_exists($value, 'toArray')) {
            return json_encode($value->toArray(), JSON_UNESCAPED_UNICODE);
        }

        return json_encode([], JSON_UNESCAPED_UNICODE);
    }

    public function supersedeByItem(int $itemId): void
    {
        DB::table('analysis_results')
            ->where('item_id', $itemId)
            ->where('status', 'active')
            ->update([
                'status'      => 'superseded',
                'updated_at'  => now(),
            ]);
    }

    public function findByRequestId(int $analysisRequestId): ?AnalysisResult
    {
        $row = DB::table('analysis_results')
            ->where('analysis_request_id', $analysisRequestId)
            ->orderByDesc('id')
            ->first();

        if (! $row) {
            return null;
        }

        $tokens = $row->classified_tokens
    ? json_decode($row->classified_tokens, true)
    : null;

$confidenceMap = $row->confidence_map
    ? json_decode($row->confidence_map, true)
    : [];

$evidence = $row->evidence
    ? json_decode($row->evidence, true)
    : null;

return AnalysisResult::reconstruct(
    requestId: (int)$row->analysis_request_id,
    brandName: $row->brand_name,
    conditionName: $row->condition_name,
    colorName: $row->color_name,
    classifiedTokens: $tokens,          // ✅
    confidenceMap: $confidenceMap,       // ✅
    overallConfidence: $row->overall_confidence !== null
        ? (float)$row->overall_confidence
        : null,
    evidence: $evidence,
    status: (string)$row->status,
    createdAt: new \DateTimeImmutable($row->created_at),
);

    }

    public function markRejectedByRequestId(int $requestId): void
{
    DB::table('analysis_results')
        ->where('analysis_request_id', $requestId)
        ->update([
            'status' => 'rejected',
            'updated_at' => now(),
        ]);
}
}