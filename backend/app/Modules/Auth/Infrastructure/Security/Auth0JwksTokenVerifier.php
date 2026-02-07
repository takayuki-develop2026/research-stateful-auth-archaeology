<?php

namespace App\Modules\Auth\Infrastructure\Security;

use App\Modules\Auth\Domain\Dto\DecodedToken;
use App\Modules\Auth\Domain\Port\TokenVerifierPort;
use Firebase\JWT\JWT;
use Firebase\JWT\Key;

final class Auth0JwksTokenVerifier implements TokenVerifierPort
{
    private readonly string $jwksUrl;

    private const LEEWAY_SECONDS = 60;
    private const FETCH_TIMEOUT_SECONDS = 5;

    // TTLの安全上限・下限（秒）
    private const MIN_JWKS_TTL = 60;
    private const MAX_JWKS_TTL = 86400;

    public function __construct(
    private readonly string $domain,
    private readonly string $audience,
    private readonly ?string $issuer = null,
) {
    if (trim($this->domain) === '' || trim($this->audience) === '') {
        throw new \RuntimeException('Auth0JwksTokenVerifier: domain/audience required');
    }
    $this->jwksUrl = 'https://' . $this->domain . '/.well-known/jwks.json';
}

    public function decode(string $jwt): DecodedToken
    {
        if (substr_count($jwt, '.') !== 2) {
            throw new \UnexpectedValueException('Auth0 token format invalid (must have two dots)');
        }

        [$h64] = explode('.', $jwt, 2);
        $header = json_decode($this->b64urlDecode($h64), true);

        if (!is_array($header) || empty($header['kid'])) {
            throw new \UnexpectedValueException('Auth0 token header missing kid');
        }
        if (($header['alg'] ?? '') !== 'RS256') {
            throw new \UnexpectedValueException('Auth0 token alg must be RS256');
        }

        $kid = (string) $header['kid'];

        // 1) cache
        $jwks = $this->fetchJwks(forceRefresh: false);
        $pem  = $this->findPemByKid($jwks, $kid);

        // 2) kidが無ければ強制リフレッシュして再試行（鍵ローテ想定）
        if ($pem === null) {
            $jwks = $this->fetchJwks(forceRefresh: true);
            $pem  = $this->findPemByKid($jwks, $kid);
        }

        if ($pem === null) {
            throw new \UnexpectedValueException("Auth0 public key not found for kid: {$kid}");
        }

        $prevLeeway = JWT::$leeway ?? 0;
        JWT::$leeway = self::LEEWAY_SECONDS;

        try {
            $payload = JWT::decode($jwt, new Key($pem, 'RS256'));
        } finally {
            JWT::$leeway = $prevLeeway;
        }

        // iss/aud 検証（末尾/揺れ吸収）
        $iss = (string) ($payload->iss ?? '');
        $aud = $payload->aud ?? null;

        $expectedIss = $this->issuer ?? ('https://' . $this->domain . '/');
        $issNorm = rtrim($iss, '/') . '/';
        $expectedIssNorm = rtrim($expectedIss, '/') . '/';

        if ($issNorm !== $expectedIssNorm) {
            throw new \UnexpectedValueException("Invalid iss: {$iss}");
        }

        // aud は string or array の両対応
        $audOk = false;
        if (is_string($aud)) {
            $audOk = ($aud === $this->audience);
        } elseif (is_array($aud)) {
            $audOk = in_array($this->audience, $aud, true);
        }
        if (!$audOk) {
            $audDump = is_array($aud) ? json_encode($aud, JSON_UNESCAPED_SLASHES) : (string)$aud;
            throw new \UnexpectedValueException("Invalid aud: {$audDump}");
        }

        // sub 必須（OPA連携でも主語になる）
        $sub = (string)($payload->sub ?? '');
        if ($sub === '') {
            throw new \UnexpectedValueException('Auth0 token missing sub');
        }

        return new DecodedToken(
            provider: 'auth0',
            payload: $payload
        );
    }

    /** @return array<string,mixed> */
    private function fetchJwks(bool $forceRefresh): array
    {
        $cacheKey = md5($this->domain);
        $cacheFile = sys_get_temp_dir() . "/auth0_jwks_{$cacheKey}.json";
        $metaFile  = sys_get_temp_dir() . "/auth0_jwks_{$cacheKey}.meta.json";

        $now = time();

        if (!$forceRefresh && is_file($cacheFile) && is_file($metaFile)) {
            $meta = json_decode((string)file_get_contents($metaFile), true);
            $expiresAt = is_array($meta) ? (int)($meta['expires_at'] ?? 0) : 0;

            if ($expiresAt > $now) {
                $json = (string) file_get_contents($cacheFile);
                $jwks = json_decode($json, true);
                if (is_array($jwks)) return $jwks;
                // 壊れてたら取り直しへ
            }
        }

        [$json, $ttl] = $this->httpGetWithMaxAge($this->jwksUrl);

        file_put_contents($cacheFile, $json, LOCK_EX);
        file_put_contents($metaFile, json_encode([
            'expires_at' => $now + $ttl,
            'fetched_at' => $now,
        ]), LOCK_EX);

        $jwks = json_decode($json, true);
        if (!is_array($jwks)) {
            throw new \UnexpectedValueException('Invalid jwks response');
        }
        return $jwks;
    }

    /**
     * @return array{0:string,1:int} body, ttlSeconds
     */
    private function httpGetWithMaxAge(string $url): array
    {
        $ctx = stream_context_create([
            'http' => [
                'timeout' => self::FETCH_TIMEOUT_SECONDS,
                'ignore_errors' => true,
                'header' => "User-Agent: occore-api\r\n",
            ],
            'ssl' => [
                'verify_peer' => true,
                'verify_peer_name' => true,
            ],
        ]);

        $http_response_header = null;

        $body = @file_get_contents($url, false, $ctx);
    if ($body === false || $body === '') {
        throw new \UnexpectedValueException('Failed to fetch Auth0 JWKS (timeout or network)');
    }

    $ttl = 3600;

    // ✅ グローバルの $http_response_header を参照
    $headers = $http_response_header ?? null;
    if (is_array($headers)) {
        foreach ($headers as $h) {
            if (stripos($h, 'Cache-Control:') === 0) {
                if (preg_match('/max-age=(\d+)/i', $h, $m)) {
                    $ttl = (int) $m[1];
                    break;
                }
            }
        }
    }

    $ttl = max(self::MIN_JWKS_TTL, min(self::MAX_JWKS_TTL, $ttl));
    return [$body, $ttl];
}

    /**
     * @param array<string,mixed> $jwks
     */
    private function findPemByKid(array $jwks, string $kid): ?string
    {
        $keys = $jwks['keys'] ?? null;
        if (!is_array($keys)) return null;

        foreach ($keys as $k) {
            if (!is_array($k)) continue;
            if (($k['kid'] ?? '') !== $kid) continue;

            // Auth0 JWKS は RSA / RS256 が通常
            if (($k['kty'] ?? '') !== 'RSA') continue;

            $n = $k['n'] ?? null;
            $e = $k['e'] ?? null;
            if (!is_string($n) || !is_string($e)) {
                throw new \UnexpectedValueException("Auth0 jwks key missing n/e for kid: {$kid}");
            }

            return $this->rsaJwkToPem($n, $e);
        }
        return null;
    }

    private function rsaJwkToPem(string $nB64u, string $eB64u): string
    {
        $n = $this->b64urlDecode($nB64u);
        $e = $this->b64urlDecode($eB64u);

        $modulus  = $this->asn1Integer($n);
        $exponent = $this->asn1Integer($e);
        $seq = $this->asn1Sequence($modulus . $exponent);

        // rsaEncryption OID + NULL
        $rsaOid = hex2bin('300d06092a864886f70d0101010500');
        $bitString = "\x03" . $this->asn1Length(strlen($seq) + 1) . "\x00" . $seq;
        $spki = $this->asn1Sequence($rsaOid . $bitString);

        return "-----BEGIN PUBLIC KEY-----\n" .
            chunk_split(base64_encode($spki), 64, "\n") .
            "-----END PUBLIC KEY-----\n";
    }

    private function asn1Integer(string $bytes): string
    {
        if ($bytes === '') $bytes = "\x00";
        if ((ord($bytes[0]) & 0x80) === 0x80) {
            $bytes = "\x00" . $bytes;
        }
        return "\x02" . $this->asn1Length(strlen($bytes)) . $bytes;
    }

    private function asn1Sequence(string $bytes): string
    {
        return "\x30" . $this->asn1Length(strlen($bytes)) . $bytes;
    }

    private function asn1Length(int $len): string
    {
        if ($len < 128) return chr($len);
        $out = '';
        while ($len > 0) {
            $out = chr($len & 0xff) . $out;
            $len >>= 8;
        }
        return chr(0x80 | strlen($out)) . $out;
    }

    private function b64urlDecode(string $s): string
    {
        $s = strtr($s, '-_', '+/');
        $pad = strlen($s) % 4;
        if ($pad) $s .= str_repeat('=', 4 - $pad);
        $decoded = base64_decode($s, true);
        return $decoded === false ? '' : $decoded;
    }
}