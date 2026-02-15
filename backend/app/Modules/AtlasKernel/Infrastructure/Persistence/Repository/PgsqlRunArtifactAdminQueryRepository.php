<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Infrastructure\Persistence\Repository;

use Illuminate\Support\Facades\DB;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactAdminQueryRepository;

final class PgsqlRunArtifactAdminQueryRepository implements RunArtifactAdminQueryRepository
{
    public function __construct(
        private readonly string $connectionName = 'ak_pgsql',
    ) {}

    public function search(array $params): array
    {
        $kind  = isset($params['kind']) ? trim((string)$params['kind']) : '';
        $q     = isset($params['q']) ? trim((string)$params['q']) : '';
        $limit = (int)($params['limit'] ?? 50);
        $limit = max(1, min(200, $limit));

        [$cursorCreatedAt, $cursorId] = $this->decodeCursor($params['cursor'] ?? null);

        $qb = DB::connection($this->connectionName)
            ->table('run_artifacts')
            ->select([
                'id',
                'run_id',
                'trace_id',
                'artifact_kind',
                'schema_version',
                'created_at',
                'content_json',
            ]);

        if ($kind !== '') {
            $qb->where('artifact_kind', $kind);
        }

        if ($q !== '') {
    $isUlid  = (bool) preg_match('/^[0-9A-HJKMNP-TV-Z]{26}$/', $q);
    $isTrace = (bool) preg_match('/^[a-f0-9]{32}$/i', $q);

    $qb->where(function ($w) use ($q, $isUlid, $isTrace) {
        if ($isUlid) {
            $w->where('run_id', $q);
            return;
        }
        if ($isTrace) {
            $w->where('trace_id', $q);
            return;
        }
        $w->whereRaw('artifact_kind ILIKE ?', ['%'.$q.'%']);
    });
}

        // Keyset pagination: created_at desc, id desc
        if ($cursorCreatedAt !== null && $cursorId !== null) {
            $qb->where(function ($w) use ($cursorCreatedAt, $cursorId) {
                $w->where('created_at', '<', $cursorCreatedAt)
                  ->orWhere(function ($w2) use ($cursorCreatedAt, $cursorId) {
                      $w2->where('created_at', '=', $cursorCreatedAt)
                         ->where('id', '<', $cursorId);
                  });
            });
        }

        $qb->orderByDesc('created_at')->orderByDesc('id');

        // 1件多く取って next_cursor 判定
        $rows = $qb->limit($limit + 1)->get();

        $hasMore = $rows->count() > $limit;
        $rows = $rows->take($limit);

        $items = [];
        foreach ($rows as $r) {
            $items[] = [
                'id' => (int)$r->id,
                'run_id' => (string)$r->run_id,
                'trace_id' => (string)$r->trace_id,
                'artifact_kind' => (string)$r->artifact_kind,
                'schema_version' => (string)$r->schema_version,
                'created_at' => (string)$r->created_at,
                'content_json' => $this->toArray($r->content_json),
            ];
        }

        $nextCursor = null;
        if ($hasMore && !empty($items)) {
            $last = $items[count($items) - 1];
            $nextCursor = $this->encodeCursor($last['created_at'], $last['id']);
        }

        return [
            'items' => $items,
            'next_cursor' => $nextCursor,
        ];
    }

    private function toArray(mixed $json): array
    {
        if (is_string($json)) {
            $d = json_decode($json, true);
            return is_array($d) ? $d : [];
        }
        if (is_object($json)) {
            $d = json_decode(json_encode($json, JSON_UNESCAPED_UNICODE), true);
            return is_array($d) ? $d : [];
        }
        if (is_array($json)) {
            return $json;
        }
        return [];
    }

    /** @return array{0:?string,1:?int} */
    private function decodeCursor(?string $cursor): array
    {
        if (!$cursor) return [null, null];

        $b64 = strtr($cursor, '-_', '+/');
        $pad = strlen($b64) % 4;
        if ($pad) $b64 .= str_repeat('=', 4 - $pad);

        $raw = base64_decode($b64, true);
        if ($raw === false) return [null, null];

        $j = json_decode($raw, true);
        if (!is_array($j)) return [null, null];

        $createdAt = isset($j['created_at']) && is_string($j['created_at']) ? $j['created_at'] : null;
        $id = isset($j['id']) ? (int)$j['id'] : null;
        if (!$createdAt || !$id) return [null, null];

        return [$createdAt, $id];
    }

    private function encodeCursor(string $createdAt, int $id): string
    {
        $raw = json_encode(['created_at' => $createdAt, 'id' => $id], JSON_UNESCAPED_UNICODE);
        $b64 = base64_encode($raw ?: '');
        return rtrim(strtr($b64, '+/', '-_'), '=');
    }
}