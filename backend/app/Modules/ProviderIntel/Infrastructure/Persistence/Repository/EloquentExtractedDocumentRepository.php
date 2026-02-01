<?php

namespace App\Modules\ProviderIntel\Infrastructure\Persistence\Repository;

use App\Modules\ProviderIntel\Domain\Repository\ExtractedDocumentRepository;
use Illuminate\Database\QueryException;
use Illuminate\Support\Facades\DB;

final class EloquentExtractedDocumentRepository implements ExtractedDocumentRepository
{
    public function save(array $attrs): int
    {
        $domain = (string)($attrs['domain'] ?? '');
        $contentHash = (string)($attrs['content_hash'] ?? '');
        if ($domain === '' || $contentHash === '') {
            throw new \InvalidArgumentException('domain/content_hash is required');
        }

        $sourceUrlHash    = (string)($attrs['source_url_hash'] ?? '');
        $rawHash          = (string)($attrs['raw_hash'] ?? '');
        $engine           = (string)($attrs['engine'] ?? '');
        $mode             = (string)($attrs['mode'] ?? '');
        $lang             = (string)($attrs['lang'] ?? '');
        $pipelineVersion  = (string)($attrs['pipeline_version'] ?? 'v4.2');

        $canUseRawKey = ($sourceUrlHash !== '' && $rawHash !== '');

        // =========================================================
        // 1) v4.2.2 RAW KEY で既存を探す（最優先）
        // =========================================================
        if ($canUseRawKey) {
            $existingId = DB::table('extracted_documents')
                ->where('domain', $domain)
                ->where('source_url_hash', $sourceUrlHash)
                ->where('raw_hash', $rawHash)
                ->where('engine', $engine)
                ->where('mode', $mode)
                ->where('lang', $lang)
                ->where('pipeline_version', $pipelineVersion)
                ->value('id');

            if ($existingId) {
                return (int)$existingId;
            }
        }

        // =========================================================
        // 2) legacy(content_hash) で既存を探す（移行用）
        //    見つかったら v4.2.2列を“埋め戻し”してから返す
        // =========================================================
        $legacyRow = DB::table('extracted_documents')
            ->where('domain', $domain)
            ->where('content_hash', $contentHash)
            ->first();

        if ($legacyRow) {
            $legacyId = (int)$legacyRow->id;

            // 既存の監査を壊さない：空の列だけ埋める
            $update = [];
            if ($canUseRawKey && empty($legacyRow->raw_hash)) {
                $update['raw_hash'] = $rawHash;
            }
            if ($canUseRawKey && empty($legacyRow->source_url_hash) && $sourceUrlHash !== '') {
                $update['source_url_hash'] = $sourceUrlHash;
            }
            if (empty($legacyRow->engine) && $engine !== '') {
                $update['engine'] = $engine;
            }
            if (empty($legacyRow->mode) && $mode !== '') {
                $update['mode'] = $mode;
            }
            if (empty($legacyRow->lang) && $lang !== '') {
                $update['lang'] = $lang;
            }
            // pipeline_version は v4.2.2 の基準として固定（空なら埋める）
            if (empty($legacyRow->pipeline_version) && $pipelineVersion !== '') {
                $update['pipeline_version'] = $pipelineVersion;
            }

            if (!empty($update)) {
                $update['updated_at'] = now();
                DB::table('extracted_documents')->where('id', $legacyId)->update($update);
            }

            return $legacyId;
        }

        // =========================================================
        // 3) INSERT（v4.2.2）
        // =========================================================
        try {
            return (int) DB::table('extracted_documents')->insertGetId([
                'project_id'       => $attrs['project_id'] ?? null,
                'domain'           => $domain,
                'source_type'      => (string)($attrs['source_type'] ?? ''),
                'source_url'       => $attrs['source_url'] ?? null,
                'source_url_hash'  => $attrs['source_url_hash'] ?? null,
                'raw_hash'         => $canUseRawKey ? $rawHash : null,
                'engine'           => $engine,
                'mode'             => $mode,
                'lang'             => $lang,
                'pipeline_version' => $pipelineVersion,
                'content_text'     => (string)($attrs['content_text'] ?? ''),
                'content_hash'     => $contentHash,
                'extracted_at'     => $attrs['extracted_at'] ?? now(),
                'created_at'       => now(),
                'updated_at'       => now(),
            ]);
        } catch (QueryException $e) {
            // 競合（同じ複合ユニーク）なら取り直す
            if ((string)$e->getCode() === '23000' && $canUseRawKey) {
                $id = DB::table('extracted_documents')
                    ->where('domain', $domain)
                    ->where('source_url_hash', $sourceUrlHash)
                    ->where('raw_hash', $rawHash)
                    ->where('engine', $engine)
                    ->where('mode', $mode)
                    ->where('lang', $lang)
                    ->where('pipeline_version', $pipelineVersion)
                    ->value('id');

                if ($id) {
                    return (int)$id;
                }
            }
            throw $e;
        }
    }

    public function find(int $id): ?array
    {
        $r = DB::table('extracted_documents')->where('id', $id)->first();
        return $r ? (array)$r : null;
    }

    public function findLatestBySourceUrlHash(string $domain, string $sourceUrlHash): ?array
    {
        $r = DB::table('extracted_documents')
            ->where('domain', $domain)
            ->where('source_url_hash', $sourceUrlHash)
            ->orderByDesc('id')
            ->first();

        return $r ? (array)$r : null;
    }

    public function findLatestBySourceUrlHashExcludingId(string $domain, string $sourceUrlHash, int $excludeId): ?array
    {
        $r = DB::table('extracted_documents')
            ->where('domain', $domain)
            ->where('source_url_hash', $sourceUrlHash)
            ->where('id', '<>', $excludeId)
            ->orderByDesc('id')
            ->first();

        return $r ? (array)$r : null;
    }
}