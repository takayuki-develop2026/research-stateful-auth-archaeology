<?php

namespace App\Modules\Item\Domain\Service;

use App\Modules\Item\Infrastructure\External\HttpAtlasKernelClient;

final class AtlasKernelService
{
    public function __construct(
        private LocalDictionaryAnalyzer $local,
        private HttpAtlasKernelClient $http,
    ) {}

    /**
     * @param array<string,mixed> $context
     * @return array<string,mixed>
     */
    public function requestAnalysis(
        int $itemId,
        string $rawText,
        ?int $tenantId = null,
        array $context = []
    ): array {
        // ✅ config cache 前提
        $mode = (string) config('atlaskernel.mode', 'local');

        // ログ用（設定名はあなたのconfigに合わせて調整してOK）
        $endpointEntity = (string) (config('atlaskernel.endpoint_entity') ?? '');

        if ($mode === 'http') {
            $res = $this->http->analyze($itemId, $rawText, $tenantId, $context);
            // ✅ “HTTPを使った” が一目で分かる
            $res['source'] = $res['source'] ?? 'ai_provisional';
            return $res;
        }

        if ($mode === 'hybrid') {
            try {
                $res = $this->http->analyze($itemId, $rawText, $tenantId, $context);
                $res['source'] = $res['source'] ?? 'ai_provisional';
                return $res;
            } catch (\Throwable $e) {
                logger()->warning('[AtlasKernel] http failed, fallback to local', [
                    'mode' => $mode,
                    'endpoint_entity' => $endpointEntity,
                    'item_id' => $itemId,
                    'error' => $e->getMessage(),
                ]);

                $res = method_exists($this->local, 'analyzeWithContext')
                    ? $this->local->analyzeWithContext($itemId, $rawText, $tenantId, $context)
                    : $this->local->analyze($itemId, $rawText, $tenantId);

                // ✅ “fallbackした” が一目で分かる
                $res['source'] = $res['source'] ?? 'local_fallback';
                return $res;
            }
        }

        // local
        $res = method_exists($this->local, 'analyzeWithContext')
            ? $this->local->analyzeWithContext($itemId, $rawText, $tenantId, $context)
            : $this->local->analyze($itemId, $rawText, $tenantId);

        // ✅ local直叩きも識別できる
        $res['source'] = $res['source'] ?? 'local';
        return $res;
    }
}