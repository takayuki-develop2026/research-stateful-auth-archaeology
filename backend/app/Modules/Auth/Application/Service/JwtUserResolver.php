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

        $token = substr($authHeader, 7);

        try {
            $decoded  = $this->verifier->decode($token); // DecodedToken
            $payload  = $decoded->payload;               // object(stdClass)
            $provider = $decoded->provider;              // string
        } catch (\Throwable $e) {
            Log::warning('[JwtUserResolver] token verification failed', [
                'error' => $e->getMessage(),
            ]);
            return null;
        }

        if (!isset($payload->sub)) return null;

        $sub = (string) $payload->sub;

        $email       = $this->stringClaim($payload, 'email');
        $displayName = $this->stringClaim($payload, 'name');
        $emailVerified = $this->boolClaim($payload, 'email_verified'); // ✅ strict

        if ($provider === 'auth0') {
            // Action が付与する namespace: https://api.occore.local/
            $ns = rtrim((string) env('AUTH0_AUDIENCE', ''), '/') . '/';

            $email = $this->stringClaim($payload, "{$ns}email") ?? $email;
            $displayName = $this->stringClaim($payload, "{$ns}name") ?? $displayName;

            // ✅ ここが最重要：strict bool
            $emailVerified = $this->boolClaim($payload, "{$ns}email_verified") ?? $emailVerified;
        }

        // デバッグ：型事故を即発見できる
        Log::info('[JwtUserResolver] email_verified raw', [
            'provider' => $provider,
            'sub' => $sub,
            'email_verified' => $emailVerified,
        ]);

        try {
            $provisioned = $this->provisioning->provisionFromExternalIdentity(
                provider: $provider,
                providerUid: $sub,
                email: $email,
                emailVerified: $emailVerified, // ✅ bool|null をそのまま渡す
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
        if (!$eloquentUser) return null;

        $principal = AuthPrincipal::fromProvisionedUser(
            user: $provisioned,
            provider: $provider,
            providerUid: $sub
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
        return is_string($v) && $v !== '' ? $v : null;
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
}