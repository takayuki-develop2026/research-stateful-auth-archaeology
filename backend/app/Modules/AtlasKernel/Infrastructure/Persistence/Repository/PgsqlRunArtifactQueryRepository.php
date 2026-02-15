<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Infrastructure\Persistence\Repository;

use Illuminate\Support\Facades\DB;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactQueryRepository;

final class PgsqlRunArtifactQueryRepository implements RunArtifactQueryRepository
{
    public function __construct(
        private readonly string $connectionName = 'ak_pgsql',
    ) {}

    public function findContentByRunIdAndKind(string $runId, string $artifactKind): ?array
    {
        $row = DB::connection($this->connectionName)
            ->table('run_artifacts')
            ->select([
                'artifact_kind',
                'schema_version',
                'trace_id',
                'artifact_ref_kind',
                'artifact_ref_run_id',
                'artifact_ref_trace_id',
                'trace_trace_id',
                'content_json',
            ])
            ->where('run_id', $runId)
            ->where('artifact_kind', $artifactKind)
            ->first();

        if (!$row) {
            return null;
        }

        $decoded = $this->decodeJson($row->content_json);
        $content = is_array($decoded) ? $decoded : [];

        // ★ v1 read-contract injection (DBが真実)
        $this->injectV1Envelope(
            $content,
            artifactKind: (string)$row->artifact_kind,
            runId: $runId,
            traceId: (string)$row->trace_id,
            schemaVersion: (string)$row->schema_version,
            refKind: (string)$row->artifact_ref_kind,
            refRunId: (string)$row->artifact_ref_run_id,
            refTraceId: (string)$row->artifact_ref_trace_id,
            traceTraceId: (string)$row->trace_trace_id,
        );

        return $content;
    }

    public function listByRunId(string $runId): array
    {
        $rows = DB::connection($this->connectionName)
            ->table('run_artifacts')
            ->select([
                'artifact_kind',
                'schema_version',
                'trace_id',
                'artifact_ref_kind',
                'artifact_ref_run_id',
                'artifact_ref_trace_id',
                'trace_trace_id',
                'content_json',
                'created_at',
                'updated_at',
            ])
            ->where('run_id', $runId)
            ->orderBy('artifact_kind')
            ->get();

        $out = [];
        foreach ($rows as $r) {
            $decoded = $this->decodeJson($r->content_json);
            $content = is_array($decoded) ? $decoded : [];

            // ★ v1 read-contract injection (DBが真実)
            $this->injectV1Envelope(
                $content,
                artifactKind: (string)$r->artifact_kind,
                runId: $runId,
                traceId: (string)$r->trace_id,
                schemaVersion: (string)$r->schema_version,
                refKind: (string)$r->artifact_ref_kind,
                refRunId: (string)$r->artifact_ref_run_id,
                refTraceId: (string)$r->artifact_ref_trace_id,
                traceTraceId: (string)$r->trace_trace_id,
            );

            $out[] = [
                'artifact_kind' => (string)$r->artifact_kind,
                'schema_version' => (string)$r->schema_version,
                'trace_id' => (string)$r->trace_id,
                'artifact_ref_kind' => (string)$r->artifact_ref_kind,
                'artifact_ref_run_id' => (string)$r->artifact_ref_run_id,
                'artifact_ref_trace_id' => (string)$r->artifact_ref_trace_id,
                'trace_trace_id' => (string)$r->trace_trace_id,
                'content_json' => $content,
                'created_at' => (string)$r->created_at,
                'updated_at' => (string)$r->updated_at,
            ];
        }

        return $out;
    }

    /** @return array<string,mixed>|null */
    private function decodeJson(mixed $json): ?array
    {
        if (is_string($json)) {
            $decoded = json_decode($json, true);
            return is_array($decoded) ? $decoded : null;
        }
        if (is_object($json)) {
            $decoded = json_decode(json_encode($json, JSON_UNESCAPED_UNICODE), true);
            return is_array($decoded) ? $decoded : null;
        }
        if (is_array($json)) {
            return $json;
        }
        return null;
    }

    /**
     * v1 contract envelope injection.
     * DB列が真実。content_json はpayloadで、足りないキーを補う。
     *
     * @param array<string,mixed> $content
     */
    private function injectV1Envelope(
        array &$content,
        string $artifactKind,
        string $runId,
        string $traceId,
        string $schemaVersion,
        string $refKind,
        string $refRunId,
        string $refTraceId,
        string $traceTraceId,
    ): void {
        // schema_version
        $content['schema_version'] = $schemaVersion !== '' ? $schemaVersion : '1.0';

        // artifact_ref
        $artifactRef = is_array($content['artifact_ref'] ?? null) ? $content['artifact_ref'] : [];
        $artifactRef['kind'] = $refKind !== '' ? $refKind : $artifactKind;
        $artifactRef['run_id'] = $refRunId !== '' ? $refRunId : $runId;
        $artifactRef['trace_id'] = $refTraceId !== '' ? $refTraceId : $traceId;
        // id が無い場合は「readモデル上の便宜キー」を入れる（writerで後で正式化OK）
        $artifactRef['id'] = isset($artifactRef['id']) && is_string($artifactRef['id']) && trim($artifactRef['id']) !== ''
            ? $artifactRef['id']
            : 'db:run_artifacts';

        $content['artifact_ref'] = $artifactRef;

        // trace
        $trace = is_array($content['trace'] ?? null) ? $content['trace'] : [];
        $trace['trace_id'] = $traceTraceId !== '' ? $traceTraceId : $traceId;
        $content['trace'] = $trace;

        // produced_by（最小デフォルト）
        $producedBy = is_array($content['produced_by'] ?? null) ? $content['produced_by'] : [];
        $producedBy['type'] = isset($producedBy['type']) && is_string($producedBy['type']) && trim($producedBy['type']) !== ''
            ? $producedBy['type']
            : 'system';
        $producedBy['name'] = isset($producedBy['name']) && is_string($producedBy['name']) && trim($producedBy['name']) !== ''
            ? $producedBy['name']
            : 'ak-go-core';
        $content['produced_by'] = $producedBy;

        // policy_version / pipeline_version（最小デフォルト）
        $content['policy_version'] = isset($content['policy_version']) && is_string($content['policy_version']) && trim($content['policy_version']) !== ''
            ? $content['policy_version']
            : 'v3';
        $content['pipeline_version'] = isset($content['pipeline_version']) && is_string($content['pipeline_version']) && trim($content['pipeline_version']) !== ''
            ? $content['pipeline_version']
            : 'v3';

        // evidence_refs（無ければ空配列）
        if (!is_array($content['evidence_refs'] ?? null)) {
            $content['evidence_refs'] = [];
        }
    }
}