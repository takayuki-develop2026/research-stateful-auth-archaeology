<?php

declare(strict_types=1);

namespace App\Modules\Auth\Infrastructure\Security;

use App\Modules\Auth\Domain\Dto\DecodedToken;
use App\Modules\Auth\Domain\Port\TokenVerifierPort;

final class CompositeTokenVerifier implements TokenVerifierPort
{
    /**
     * @param array<string, TokenVerifierPort> $verifiers keyed by provider name (e.g. 'auth0', 'firebase')
     * @param string[] $order preferred order (e.g. ['auth0', 'firebase'])
     */
    public function __construct(
        private readonly array $verifiers,
        private readonly array $order,
    ) {
        if (empty($this->verifiers)) {
            throw new \InvalidArgumentException('CompositeTokenVerifier: verifiers is empty');
        }
    }

    public function decode(string $jwt): DecodedToken
    {
        $errors = [];

        $order = $this->order;
        if (empty($order)) {
            $order = array_keys($this->verifiers);
        }

        foreach ($order as $providerRaw) {
            $provider = strtolower(trim((string) $providerRaw));
            if ($provider === '') {
                continue;
            }

            $v = $this->verifiers[$provider] ?? null;
            if (!$v) {
                $errors[$provider] = 'verifier not registered';
                continue;
            }

            try {
                return $v->decode($jwt);
            } catch (\Throwable $e) {
                // 詳細はログ用途。外に返しすぎないのが安全。
                $errors[$provider] = $e->getMessage();
            }
        }

        // 返すメッセージは最小限にする（詳細は呼び出し側でログへ）
        // どうしてもここで詳細が必要なら、環境変数で debug 時だけ展開する。
        $msg = 'JWT verification failed';

        if (filter_var((string) env('AUTH_DEBUG_JWT', 'false'), FILTER_VALIDATE_BOOL)) {
            $msg .= ': ' . json_encode($errors, JSON_UNESCAPED_SLASHES | JSON_PARTIAL_OUTPUT_ON_ERROR);
        }

        throw new \UnexpectedValueException($msg);
    }
}