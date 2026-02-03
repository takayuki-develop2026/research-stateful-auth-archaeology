<?php

namespace App\Modules\Item\Domain\Service;

use DomainException;

/**
 * 辞書ファイル（Textファイル）を基にアイテムの解析を行うクラス
 */
final class LocalDictionaryAnalyzer
{
    private string $assetPath;

    public function __construct()
    {
        $this->assetPath = config('atlaskernel.assets_path');
    }

    /**
     * ✅ v3推奨：context 入口（後方互換100%）
     * - hybrid fallback でも brand_text を活かす
     * - Localは順不同耐性が弱いので "ヒント注入" だけでも価値がある
     *
     * @param array<string,mixed> $context
     */
    public function analyzeWithContext(
        int $itemId,
        string $rawText,
        ?int $tenantId = null,
        array $context = []
    ): array {
        $brandText = $context['brand_text'] ?? null;

        if (is_string($brandText) && trim($brandText) !== '') {
            // “ヒント”として rawText の先頭に差し込む（最小で効く）
            $rawText = $brandText . ' ' . $rawText;
        }

        return $this->analyze($itemId, $rawText, $tenantId);
    }

    /**
     * 解析のメイン処理（AtlasKernelServiceから移植）
     */
    public function analyze(int $itemId, string $rawText, ?int $tenantId = null): array
    {
        if ($itemId <= 0) {
            throw new DomainException('Invalid itemId');
        }

        $text = $this->normalize($rawText);

        // ======================================================
        // Dict load
        // ======================================================
        $brandDict         = $this->loadDict('brands_v1.txt');
        $brandAliasMap     = $this->loadGroupedAliasMap('brands_v1.txt');

        $conditionDict     = $this->loadDict('conditions_v1.txt');
        $colorDict         = $this->loadDict('colors_v1.txt');

        $conditionAliasMap = $this->loadGroupedAliasMap('conditions_v1.txt');
        $colorAliasMap     = $this->loadGroupedAliasMap('colors_v1.txt');

        // ======================================================
        // Extraction（抽出）
        // ======================================================
        [$conditionRaw, $text, $confCondition] = $this->extractOne($text, $conditionDict);
        [$colorRaw, $text, $confColor]         = $this->extractOne($text, $colorDict);
        [$brandsRaw, $text]                    = $this->extractMany($text, $brandDict);

        // ======================================================
        // Canonicalize（正規化・名寄せ）
        // ======================================================
        // ✅ 必須修正：alias map lookup は normalize(key) で揃える
        $condition = $conditionRaw
            ? ($conditionAliasMap[$this->normalize($conditionRaw)] ?? $conditionRaw)
            : null;

        $color = $colorRaw
            ? ($colorAliasMap[$this->normalize($colorRaw)] ?? $colorRaw)
            : null;

        $brandsCanonical = [];
        foreach ($brandsRaw as $b) {
            $bn = $this->normalize((string) $b);
            if ($bn === '') {
                continue;
            }
            // brand は normalize 済みキーで map を引く（mapも normalize 済み）
            $brandsCanonical[] = $brandAliasMap[$bn] ?? $bn;
        }
        $brandsCanonical = array_values(array_unique($brandsCanonical));
        $brandName = $brandsCanonical[0] ?? null;

        // ======================================================
        // Tags 生成（従来どおり複数保持）
        // ======================================================
        $tags = [];

        foreach ($brandsCanonical as $b) {
            $tags['brand'][] = [
                'display_name' => $b,
                'raw_token'    => null,
                'entity_id'    => null,
                'confidence'   => 0.9,
            ];
        }

        if ($condition) {
            $tags['condition'][] = [
                'display_name' => $condition,
                'raw_token'    => $conditionRaw,
                'entity_id'    => null,
                'confidence'   => $confCondition,
            ];
        }

        if ($color) {
            $tags['color'][] = [
                'display_name' => $color,
                'raw_token'    => $colorRaw,
                'entity_id'    => null,
                'confidence'   => $confColor,
            ];
        }

        $confidenceMap = [
            'brand'     => $brandName ? 0.9 : 0.0,
            'condition' => $confCondition ?? 0.0,
            'color'     => $confColor ?? 0.0,
        ];

        $overallConfidence = max(
            $confidenceMap['brand'],
            $confidenceMap['condition'],
            $confidenceMap['color']
        );

        // ✅ v3監査：best_tok を必ず tokens に残す（brand/condition/colorを「1本」に揃える）
        $bestBrandTok = isset($brandsRaw[0]) ? (string) $brandsRaw[0] : null;

        return [
            'brand' => [
                'name'       => $brandName,
                'confidence' => $confidenceMap['brand'],
            ],
            'condition' => [
                'name'       => $condition,
                'confidence' => $confidenceMap['condition'],
            ],
            'color' => [
                'name'       => $color,
                'confidence' => $confidenceMap['color'],
            ],
            'tokens' => [
                'brand'     => ($bestBrandTok && trim($bestBrandTok) !== '') ? [$bestBrandTok] : [],
                'condition' => $conditionRaw ? [(string) $conditionRaw] : [],
                'color'     => $colorRaw ? [(string) $colorRaw] : [],
            ],
            'tags' => $tags,
            'confidence_map'     => $confidenceMap,
            'overall_confidence' => $overallConfidence,
        ];
    }

    /* --- 以下、Helperメソッド群の移植 --- */

    private function normalize(string $text): string
    {
        $text = trim($text);
        $text = mb_convert_kana($text, 'asVC', 'UTF-8');
        return trim(preg_replace('/\s+/u', ' ', $text) ?? $text);
    }

    private function loadDict(string $file): array
    {
        $path = "{$this->assetPath}/{$file}";
        if (!file_exists($path)) {
            return [];
        }

        return array_values(array_unique(array_map(
            fn ($v) => $this->normalize((string) $v),
            file($path, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES) ?: []
        )));
    }

    /**
     * brands_v1.txt / conditions_v1.txt / colors_v1.txt の想定:
     *   canonical
     *   alias1
     *   alias2
     *
     *   canonical2
     *   alias...
     */
    private function loadGroupedAliasMap(string $file): array
    {
        $map = [];
        $path = "{$this->assetPath}/{$file}";
        if (!file_exists($path)) {
            return $map;
        }

        $lines = file($path, FILE_IGNORE_NEW_LINES);
        $currentCanonical = null;

        foreach ($lines ?: [] as $line) {
            $trimmed = trim((string) $line);
            if ($trimmed === '') {
                $currentCanonical = null;
                continue;
            }

            $v = $this->normalize($trimmed);
            if ($v === '') {
                continue;
            }

            if ($currentCanonical === null) {
                $currentCanonical = $v;
                $map[$currentCanonical] = $currentCanonical;
                continue;
            }

            $map[$v] = $currentCanonical;
        }

        return $map;
    }

    private function extractOne(string $text, array $dict): array
    {
        if (empty($dict)) {
            return [null, $text, 0.0];
        }

        usort($dict, fn ($a, $b) => mb_strlen($b) <=> mb_strlen($a));

        foreach ($dict as $word) {
            if ($word !== '' && mb_strpos($text, $word) !== false) {
                $confidence = min(1.0, 0.5 + mb_strlen($word) / max(mb_strlen($text), 1));
                return [$word, $this->normalize(str_replace($word, '', $text)), $confidence];
            }
        }

        return [null, $text, 0.0];
    }

    private function extractMany(string $text, array $dict): array
    {
        if (empty($dict)) {
            return [[], $text];
        }

        $found = [];
        usort($dict, fn ($a, $b) => mb_strlen($b) <=> mb_strlen($a));

        foreach ($dict as $word) {
            if ($word !== '' && mb_strpos($text, $word) !== false) {
                $found[] = $word;
                $text = str_replace($word, '', $text);
            }
        }

        return [array_values(array_unique($found)), $this->normalize($text)];
    }
}
