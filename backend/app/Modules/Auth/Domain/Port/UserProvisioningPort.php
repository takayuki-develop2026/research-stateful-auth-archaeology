<?php

namespace App\Modules\Auth\Domain\Port;

use App\Modules\Auth\Domain\Dto\ProvisionedUser;

interface UserProvisioningPort
{
    /**
     * ✅ 全方式共通：外部ID（provider + sub）で User を確定する唯一の入口
     *
     * ルール（不変）:
     * - 主キーは (provider, provider_uid)
     * - email は “補助キー”。email だけで既存 user を拾うのは厳格条件下のみ（後述）
     * - verified は分離テーブルへ記録（users.email_verified_at を直接SoTにしない）
     */
    public function provisionFromExternalIdentity(
        string $provider,        // 'firebase' | 'auth0' | 'cognito' | 'custom' | 'token'
        string $providerUid,     // OIDC sub / Firebase uid / etc
        ?string $email = null,
        ?bool $emailVerified = null,
        ?string $displayName = null,
        array $claims = [],
    ): ProvisionedUser;

    /**
     * ✅ 互換維持（既存JWT: sub=内部user_id）
     */
    public function provisionFromJwt(int $userId): ProvisionedUser;
}