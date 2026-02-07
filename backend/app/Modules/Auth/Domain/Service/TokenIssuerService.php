<?php

namespace App\Modules\Auth\Domain\Service;

use App\Modules\Auth\Domain\Dto\ProvisionedUser;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use Firebase\JWT\JWT;

final class TokenIssuerService
{
    private string $secret;
    private string $issuer;
    private int $ttl;

    public function __construct()
    {
        $this->secret = config('jwt.secret');
        $this->issuer = config('jwt.issuer', 'omnicommerce-core');
        $this->ttl    = (int) config('jwt.ttl', 3600);
    }

    /**
     * ✅ Access JWT 発行専用（principal を真実にする）
     */
    public function issue(ProvisionedUser $user, AuthPrincipal $principal): string
    {
        $now = time();

        $payload = [
            'iss' => $this->issuer,
            'iat' => $now,
            'exp' => $now + $this->ttl,

            // 内部主語（OCC）
            'sub' => $user->userId,

            // 外部主語（差し替えの核）
            'pid' => $principal->provider(),
            'puid' => $principal->providerUid(),

            // 認証状態（principal SoT）
            'email' => $principal->email(),
            'email_verified' => $principal->isEmailVerified(),

            // テナント/権限
            'roles' => $user->roles ?? [],
            'shop_ids' => $user->shopIds ?? [],
            'shop_id' => $user->tenantId,
        ];

        return JWT::encode($payload, $this->secret, 'HS256');
    }
}