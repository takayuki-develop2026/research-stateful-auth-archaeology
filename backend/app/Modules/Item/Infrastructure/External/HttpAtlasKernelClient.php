<?php

namespace App\Modules\Item\Infrastructure\External;

use Illuminate\Support\Facades\Http;
use RuntimeException;

final class HttpAtlasKernelClient
{
    /**
     * HTTPでAtlasKernelへ解析依頼し、Laravel側が期待する「ローカル形式」に整形して返す。
     *
     * @param array<string,mixed> $context
     * @return array{
     *   brand: array{name: ?string, confidence: float},
     *   condition: array{name: ?string, confidence: float},
     *   color: array{name: ?string, confidence: float},
     *   tokens: array{brand: array<int,string>, condition: array<int,string>, color: array<int,string>},
     *   confidence_map: array{brand: float, condition: float, color: float},
     *   overall_confidence: float
     * }
     */
    public function analyze(
        int $itemId,
        string $rawText,
        ?int $tenantId = null,
        array $context = []
    ): array {
        $timeout = (int) (config('atlaskernel.timeout') ?? 10);

        // ------------------------------------------------------
        // ✅ endpoint確定（最重要）
        // - 新: atlaskernel.endpoint_entity = http://python_atlaskernel:8000/v1/analyze/entity
        // - 旧: atlaskernel.endpoint (古い /v1/analyze など)
        // ------------------------------------------------------
        $endpoint = (string) (config('atlaskernel.endpoint_entity') ?? config('atlaskernel.endpoint') ?? '');
        $endpoint = trim($endpoint);

        if ($endpoint === '') {
            throw new RuntimeException('AtlasKernel endpoint is empty. Set atlaskernel.endpoint_entity.');
        }

        // 「/v1/analyze/entity」が含まれていない場合は、末尾補正（壊さず増築）
        // ※ 旧設定が "http://python_atlaskernel:8000/v1/analyze" でも救う
        if (!str_contains($endpoint, '/v1/analyze/entity')) {
            $endpoint = rtrim($endpoint, '/');
            // 末尾が /v1/analyze なら置換、そうでなければ append
            if (str_ends_with($endpoint, '/v1/analyze')) {
                $endpoint = substr($endpoint, 0, -strlen('/v1/analyze')) . '/v1/analyze/entity';
            } else {
                $endpoint = $endpoint . '/v1/analyze/entity';
            }
        }

// ------------------------------------------------------
// ✅ payload（Python側の期待に合わせる）
// ------------------------------------------------------
$payload = [
    'project_id' => (string) config('atlaskernel.project_id', 'occore'),
    'task_type'  => 'entity_extract',
    'raw_text'   => $rawText,
    'mode'       => 2,
    'context'    => array_merge([
        'tenant_id' => $tenantId,
        'item_id'   => $itemId,
    ], $context),
];

logger()->info('[🔥AtlasKernelHTTP] request', [
    'endpoint'     => $endpoint,
    'item_id'      => $itemId,
    'raw_text'     => $payload['raw_text'],
    'context_keys' => array_keys($payload['context']),
    'brand_text'   => $payload['context']['brand_text'] ?? null, // あれば出す
]);


        $res = Http::timeout($timeout)
            ->acceptJson()
            ->asJson()
            ->post($endpoint, $payload);

        if (!$res->ok()) {
            throw new RuntimeException(
                'AtlasKernel HTTP failed: ' . $res->status() . ' ' . $res->body() . ' endpoint=' . $endpoint
            );
        }

        $json = $res->json();
        if (!is_array($json)) {
            throw new RuntimeException('AtlasKernel HTTP invalid response: non-json object');
        }

        // ------------------------------------------------------
        // ✅ レスポンス吸収（壊さず増築）
        // 1) 新フラット形式（あなたの curl 結果）
        //    { brand:{name}, condition:{name}, color:{name}, confidence_map, overall_confidence, tokens }
        // 2) 旧形式（result.items）
        // ------------------------------------------------------

        // --- 新: フラット形式 ---
        $flatBrandName = data_get($json, 'brand.name');
        $flatCondName  = data_get($json, 'condition.name');
        $flatColorName = data_get($json, 'color.name');

        $flatBrandConf = (float) (data_get($json, 'confidence_map.brand') ?? data_get($json, 'brand.confidence') ?? 0.0);
        $flatCondConf  = (float) (data_get($json, 'confidence_map.condition') ?? data_get($json, 'condition.confidence') ?? 0.0);
        $flatColorConf = (float) (data_get($json, 'confidence_map.color') ?? data_get($json, 'color.confidence') ?? 0.0);

        $flatTokensBrand = data_get($json, 'tokens.brand', []);
        $flatTokensCond  = data_get($json, 'tokens.condition', []);
        $flatTokensColor = data_get($json, 'tokens.color', []);

        $hasFlat = is_string($flatBrandName) || is_string($flatCondName) || is_string($flatColorName);

        if ($hasFlat) {
            $confidenceMap = [
                'brand'     => is_string($flatBrandName) && $flatBrandName !== '' ? $flatBrandConf : 0.0,
                'condition' => is_string($flatCondName) && $flatCondName !== '' ? $flatCondConf : 0.0,
                'color'     => is_string($flatColorName) && $flatColorName !== '' ? $flatColorConf : 0.0,
            ];
            $overall = (float) (data_get($json, 'overall_confidence') ?? max($confidenceMap['brand'], $confidenceMap['condition'], $confidenceMap['color']));

            return [
                'brand' => [
                    'name'       => is_string($flatBrandName) ? $flatBrandName : null,
                    'confidence' => $confidenceMap['brand'],
                ],
                'condition' => [
                    'name'       => is_string($flatCondName) ? $flatCondName : null,
                    'confidence' => $confidenceMap['condition'],
                ],
                'color' => [
                    'name'       => is_string($flatColorName) ? $flatColorName : null,
                    'confidence' => $confidenceMap['color'],
                ],
                'tokens' => [
                    'brand'     => $this->sanitizeTokens($flatTokensBrand),
                    'condition' => $this->sanitizeTokens($flatTokensCond),
                    'color'     => $this->sanitizeTokens($flatTokensColor),
                ],
                'confidence_map'     => $confidenceMap,
                'overall_confidence' => $overall,
            ];
        }

        // --- 旧: result.items 形式 ---
        $items = data_get($json, 'result.items', []);
        if (!is_array($items)) {
            throw new RuntimeException('AtlasKernel HTTP invalid response: neither flat nor result.items');
        }

        $brand     = $this->findByType($items, 'brand');
        $condition = $this->findByType($items, 'condition');
        $color     = $this->findByType($items, 'color');

        $brandName = is_array($brand) ? ($brand['canonical_value'] ?? null) : null;
        $condName  = is_array($condition) ? ($condition['canonical_value'] ?? null) : null;
        $colorName = is_array($color) ? ($color['canonical_value'] ?? null) : null;

        $brandConf = is_array($brand) && isset($brand['confidence']) ? (float) $brand['confidence'] : 0.0;
        $condConf  = is_array($condition) && isset($condition['confidence']) ? (float) $condition['confidence'] : 0.0;
        $colorConf = is_array($color) && isset($color['confidence']) ? (float) $color['confidence'] : 0.0;

        $confidenceMap = [
            'brand'     => $brandName ? $brandConf : 0.0,
            'condition' => $condName ? $condConf : 0.0,
            'color'     => $colorName ? $colorConf : 0.0,
        ];

        $overall = max($confidenceMap['brand'], $confidenceMap['condition'], $confidenceMap['color']);

        return [
            'brand' => [
                'name'       => $brandName,
                'confidence' => $confidenceMap['brand'],
            ],
            'condition' => [
                'name'       => $condName,
                'confidence' => $confidenceMap['condition'],
            ],
            'color' => [
                'name'       => $colorName,
                'confidence' => $confidenceMap['color'],
            ],
            'tokens' => [
                'brand'     => (is_array($brand) && isset($brand['raw_value'])) ? [(string) $brand['raw_value']] : [],
                'condition' => (is_array($condition) && isset($condition['raw_value'])) ? [(string) $condition['raw_value']] : [],
                'color'     => (is_array($color) && isset($color['raw_value'])) ? [(string) $color['raw_value']] : [],
            ],
            'confidence_map'     => $confidenceMap,
            'overall_confidence' => (float) $overall,
        ];
    }

    /** @param array<int, mixed> $items */
    private function findByType(array $items, string $type): ?array
    {
        foreach ($items as $it) {
            if (is_array($it) && ($it['entity_type'] ?? null) === $type) {
                return $it;
            }
        }
        return null;
    }

    /** @param mixed $tokens */
    private function sanitizeTokens(mixed $tokens): array
    {
        if (!is_array($tokens)) {
            return [];
        }
        $out = [];
        foreach ($tokens as $t) {
            $s = trim((string) $t);
            if ($s !== '') {
                $out[] = $s;
            }
        }
        return $out;
    }
}