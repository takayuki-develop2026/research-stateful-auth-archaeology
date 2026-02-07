<?php

namespace App\Modules\Auth\Application\Service;

use App\Modules\Auth\Domain\Port\UserProvisioningPort;
use App\Modules\Auth\Domain\Port\TokenVerifierPort;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;
use App\Models\User;

final class JwtUserResolver
{
    public function __construct(
        private TokenVerifierPort $verifier,
        private UserProvisioningPort $provisioning,
    ) {}

    public function resolve(Request $request): ?array
    {
        $authHeader = $request->header('Authorization');
        if (!is_string($authHeader) || !str_starts_with($authHeader, 'Bearer ')) {
            return null;
        }

        $token = trim(substr($authHeader, 7));
        if ($token === '') {
            return null;
        }

        try {
            $decoded  = $this->verifier->decode($token); // DecodedToken(provider, payload)
            $payload  = $decoded->payload;               // object(stdClass)
            $provider = (string) $decoded->provider;     // string
        } catch (\Throwable $e) {
            [$kid, $iss, $aud] = $this->peekJwtMeta($token);

            Log::warning('[JwtUserResolver] token verification failed', [
                'error' => $e->getMessage(),
                'kid' => $kid,
                'iss' => $iss,
                'aud' => $aud,
                // 期待値（環境不整合を即確定させる）
                'expected_issuer' => env('AUTH0_ISSUER'),
                'expected_audience' => env('AUTH0_AUDIENCE'),
                'auth0_domain' => env('AUTH0_DOMAIN'),
                'jwt_providers' => env('JWT_PROVIDERS'),
            ]);

            return null;
        }

        if (!isset($payload->sub)) {
            Log::warning('[JwtUserResolver] token payload missing sub', [
                'provider' => $provider,
            ]);
            return null;
        }

        $sub = (string) $payload->sub;
        if ($sub === '') {
            Log::warning('[JwtUserResolver] token sub empty', [
                'provider' => $provider,
            ]);
            return null;
        }

        // 標準claim
        $email         = $this->stringClaim($payload, 'email');
        $displayName   = $this->stringClaim($payload, 'name');
        $emailVerified = $this->boolClaim($payload, 'email_verified'); // strict

        // provider別：Auth0 Action namespace claim を上書き
        if ($provider === AuthPrincipal::PROVIDER_AUTH0 || $provider === 'auth0') {
            $audience = rtrim((string) env('AUTH0_AUDIENCE', ''), '/');
            $ns = $audience !== '' ? ($audience . '/') : '';

            if ($ns !== '') {
                $email       = $this->stringClaim($payload, "{$ns}email") ?? $email;
                $displayName = $this->stringClaim($payload, "{$ns}name") ?? $displayName;
                $emailVerified = $this->boolClaim($payload, "{$ns}email_verified") ?? $emailVerified;
            } else {
                // ここに来るのは env が壊れてる可能性大
                Log::warning('[JwtUserResolver] AUTH0_AUDIENCE empty; cannot read namespaced claims', [
                    'provider' => $provider,
                ]);
            }
        }

        Log::info('[JwtUserResolver] resolved claims', [
            'provider' => $provider,
            'sub' => $sub,
            'email' => $email,
            'email_verified' => $emailVerified,
        ]);

        try {
            $provisioned = $this->provisioning->provisionFromExternalIdentity(
                provider: $provider,
                providerUid: $sub,
                email: $email,
                emailVerified: $emailVerified, // bool|null
                displayName: $displayName,
                claims: get_object_vars($payload),
            );
        } catch (\Throwable $e) {
            Log::warning('[JwtUserResolver] provisioning failed', [
                'provider' => $provider,
                'sub' => $sub,
                'email' => $email,
                'error' => $e->getMessage(),
            ]);
            return null;
        }

        // 互換：古い sub=内部user_id のJWT
        if ((!$provisioned->userId) && ctype_digit($sub)) {
            $provisioned = $this->provisioning->provisionFromJwt((int) $sub);
        }

        $eloquentUser = User::find($provisioned->userId);
        if (!$eloquentUser) {
            Log::warning('[JwtUserResolver] user not found after provisioning', [
                'provider' => $provider,
                'sub' => $sub,
                'user_id' => $provisioned->userId,
            ]);
            return null;
        }

        $principal = AuthPrincipal::fromProvisionedUser(
            user: $provisioned,
            provider: $provider,
            providerUid: $sub,
        );

        return [
            'user' => $eloquentUser,
            'principal' => $principal,
        ];
    }

    private function claim(object $payload, string $key): mixed
    {
        return property_exists($payload, $key) ? $payload->{$key} : null;
    }

    private function stringClaim(object $payload, string $key): ?string
    {
        $v = $this->claim($payload, $key);
        return is_string($v) && trim($v) !== '' ? trim($v) : null;
    }

    /**
     * ✅ "false" を true にしない strict bool
     */
    private function boolClaim(object $payload, string $key): ?bool
    {
        $v = $this->claim($payload, $key);

        if (is_bool($v)) return $v;

        if (is_int($v)) {
            if ($v === 1) return true;
            if ($v === 0) return false;
            return null;
        }

        if (is_string($v)) {
            $s = strtolower(trim($v));
            if ($s === 'true' || $s === '1') return true;
            if ($s === 'false' || $s === '0') return false;
            return null;
        }

        return null;
    }

    /**
     * 失敗原因確定用：kid/iss/aud を生で覗く（署名検証はしない）
     * @return array{0:?string,1:mixed,2:mixed}
     */
    private function peekJwtMeta(string $jwt): array
    {
        $parts = explode('.', $jwt);
        if (count($parts) < 2) return [null, null, null];

        $h = json_decode($this->b64urlDecode($parts[0]), true) ?: [];
        $c = json_decode($this->b64urlDecode($parts[1]), true) ?: [];

        $kid = is_array($h) ? ($h['kid'] ?? null) : null;
        $iss = is_array($c) ? ($c['iss'] ?? null) : null;
        $aud = is_array($c) ? ($c['aud'] ?? null) : null;

        return [
            is_string($kid) ? $kid : null,
            $iss,
            $aud,
        ];
    }

    private function b64urlDecode(string $v): string
    {
        $v = strtr($v, '-_', '+/');
        $pad = strlen($v) % 4;
        if ($pad) $v .= str_repeat('=', 4 - $pad);
        $decoded = base64_decode($v, true);
        return $decoded === false ? '' : $decoded;
    }
}