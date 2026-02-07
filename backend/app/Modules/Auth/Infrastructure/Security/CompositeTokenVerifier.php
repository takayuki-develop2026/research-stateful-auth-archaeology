<?php

declare(strict_types=1);

namespace App\Modules\Auth\Infrastructure\Security;

use App\Modules\Auth\Domain\Dto\DecodedToken;
use App\Modules\Auth\Domain\Port\TokenVerifierPort;

final class CompositeTokenVerifier implements TokenVerifierPort
{
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
            if ($provider === '') continue;

            $v = $this->verifiers[$provider] ?? null;
            if (!$v) {
                $errors[$provider] = 'verifier not registered';
                continue;
            }

            try {
                return $v->decode($jwt);
            } catch (\Throwable $e) {
                $errors[$provider] = $e->getMessage();
            }
        }

        $msg = 'JWT verification failed';

        // ✅ local は常に詳細（運用では抑制）
        if (app()->environment('local') || filter_var((string) env('AUTH_DEBUG_JWT', 'false'), FILTER_VALIDATE_BOOL)) {
            $msg .= ': ' . json_encode($errors, JSON_UNESCAPED_SLASHES | JSON_PARTIAL_OUTPUT_ON_ERROR);
        }

        throw new \UnexpectedValueException($msg);
    }
}