<?php

namespace App\Modules\ProviderIntel\Application\UseCase\Admin\CatalogSource;

use App\Modules\ProviderIntel\Application\Service\ProviderIntelFetcher;
use App\Modules\ProviderIntel\Domain\Repository\CatalogSourceRepository;
use App\Modules\Review\Application\UseCase\EnqueueReviewQueueItemUseCase;

use App\Modules\ProviderIntel\Application\Service\HtmlTextExtractor;
use App\Modules\ProviderIntel\Application\Service\PdfTextExtractorClient;
use App\Modules\ProviderIntel\Application\Service\SimpleDiffGenerator;

use App\Modules\ProviderIntel\Domain\Repository\ExtractedDocumentRepository;
use App\Modules\ProviderIntel\Domain\Repository\DocumentDiffRepository;

final class RunCatalogSourceUseCase
{
    public function __construct(
        private CatalogSourceRepository $sources,
        private ProviderIntelFetcher $fetcher,
        private EnqueueReviewQueueItemUseCase $enqueueReview,

        private HtmlTextExtractor $htmlExtractor,
        private PdfTextExtractorClient $pdfExtractor,

        private ExtractedDocumentRepository $docs,
        private SimpleDiffGenerator $diff,
        private DocumentDiffRepository $diffs,
    ) {}

    /**
     * force: changed=falseでも再抽出〜enqueueまで実行する（再試行用）
     * mode : OCR制御（auto | force_ocr）
     */
    public function handle(
    int $sourceId,
    bool $force = false,
    ?string $forceReason = null,
    string $mode = 'auto',   // ★追加
): array
{
    // mode normalize（不正値はautoに倒す）
    if (!in_array($mode, ['auto', 'force_ocr'], true)) {
        $mode = 'auto';
    }

    $engine = 'tesseract';   // v4.2 固定（将来 engine router へ）
    $lang   = 'jpn';         // v4.2 固定（必要なら jpn+eng）

        $source = $this->sources->find($sourceId);
        if (!$source) {
            throw new \DomainException('CatalogSource not found');
        }

        $url = $source->sourceUrl();
        if (!$url) {
            throw new \DomainException('source_url missing');
        }

        $r = $this->fetcher->fetch($url);

        // raw body hash（更新検知）
        $newHash = hash('sha256', $r['body']);
        $oldHash = $source->lastHash();
        $changed = ($oldHash === null) || !hash_equals($oldHash, $newHash);

        // ✅ changed=false かつ force=false なら完全に何もしない
        if (!$changed && !$force) {
            return [
                'changed' => false,
                'forced' => false,
                'mode' => $mode,
                'old_hash' => $oldHash,
                'new_hash' => $newHash,
                'content_type' => $r['content_type'] ?? null,
            ];
        }

        // --- ここから先は changed=true または force=true のとき必ず実行 ---

        $contentType = (string)($r['content_type'] ?? '');
        $sourceType  = (string)$source->sourceType(); // html|pdf

        $text = '';
        $extractMeta = [];

        if ($sourceType === 'pdf') {
    $ex = $this->pdfExtractor->extractWithFallbackFromPdfBytes(
        pdfBytes: (string)$r['body'],
        sourceUrl: (string)$url,
        lang: $lang,
        engine: $engine,
        mode: $mode,
    );
    $text = $ex['text'];
    $extractMeta = $ex['meta'] ?? [];
} else {
    $text = $this->htmlExtractor->extract((string)$r['body']);
    $extractMeta = [];
}

        // extracted_document（content_hashはテキストのsha256）
        $afterDocId = $this->docs->save([
    'project_id' => $source->projectId(),
    'domain' => 'providerintel',
    'source_type' => $sourceType,
    'source_url' => $source->sourceUrl(),
    'source_url_hash' => $source->sourceUrlHash(),

    // v4.2.2 key
    'raw_hash' => $newHash,
    'engine' => $sourceType === 'pdf' ? $engine : '',
    'mode' => $sourceType === 'pdf' ? $mode : '',
    'lang' => $sourceType === 'pdf' ? $lang : '',
    'pipeline_version' => 'v4.2',

    'content_text' => $text,
    'content_hash' => hash('sha256', $text),
    'extracted_at' => now(),
]);

        // before doc（同URL hashで最新、ただし自分除外）
        $before = $this->docs->findLatestBySourceUrlHashExcludingId(
            'providerintel',
            $source->sourceUrlHash(),
            $afterDocId
        );

        $beforeId = $before ? (int)($before['id'] ?? 0) : null;
        $beforeText = $before ? (string)($before['content_text'] ?? '') : null;

        // diff summary
        $diffSummary = $this->diff->summarize($beforeText, $text);
        $diffSummary = array_merge($diffSummary, [
            'source_type' => $sourceType,
            'content_type' => $contentType,
            'extract_meta' => $extractMeta, // pipeline/decision/budget等はここに入る
        ]);

        // ✅ DocumentDiffRepositoryは positional のまま（named args禁止）
        $diffId = $this->diffs->save(
            $source->projectId(),
            $beforeId,
            $afterDocId,
            $diffSummary,
        );

        // last_hashは「最新のraw body」を記録（force時も更新して良い）
        $updated = $source->withLastFetch($newHash, new \DateTimeImmutable('now'));
        $this->sources->updateLastFetch($updated);

        // enqueue review（pendingがある場合は上書き更新される前提）
        $this->enqueueReview->handle(
            projectId: $source->projectId(),
            queueType: 'providerintel',
            refType: 'catalog_source',
            refId: $source->id(),
            priority: 50,
            summary: [
                'source_id' => $source->id(),
                'provider_id' => $source->providerId(),
                'source_type' => $sourceType,
                'source_url' => $source->sourceUrl(),
                'old_hash' => $oldHash,
                'new_hash' => $newHash,
                'content_type' => $contentType,
                'after_document_id' => $afterDocId,
                'diff_id' => $diffId,

                // ✅ v4.2.1 “後戻りゼロ” 監査キー
                'forced' => $force,
                'force_reason' => $force ? ($forceReason ?? 'manual_force') : null,
                'mode' => $mode,
            ]
        );

        return [
            'changed' => $changed,
            'forced' => $force,
            'mode' => $mode,
            'old_hash' => $oldHash,
            'new_hash' => $newHash,
            'content_type' => $contentType,
            'after_document_id' => $afterDocId,
            'diff_id' => $diffId,
        ];
    }
}