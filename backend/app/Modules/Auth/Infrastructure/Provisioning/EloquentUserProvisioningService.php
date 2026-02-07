<?php

namespace App\Modules\Auth\Infrastructure\Provisioning;

use App\Models\User;
use App\Models\UserIdentity;
use App\Models\UserIdentityVerification;
use App\Modules\Auth\Domain\Port\UserProvisioningPort;
use App\Modules\Auth\Domain\Dto\ProvisionedUser;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Auth\AuthenticationException;

final class EloquentUserProvisioningService implements UserProvisioningPort
{
    public function provisionFromExternalIdentity(
        string $provider,
        string $providerUid,
        ?string $email = null,
        ?bool $emailVerified = null,
        ?string $displayName = null,
        array $claims = [],
    ): ProvisionedUser {
        return DB::transaction(function () use (
            $provider, $providerUid, $email, $emailVerified, $displayName, $claims
        ) {
            // 1) provider+uid で探す
            $identity = UserIdentity::where('provider', $provider)
                ->where('provider_uid', $providerUid)
                ->first();

            if ($identity) {
                $user = User::find($identity->user_id);
                if (!$user) {
                    throw new \RuntimeException("user not found for identity: {$identity->id}");
                }

                // claims などを最新化（任意）
                $identity->email = $email ?? $identity->email;
                $identity->display_name = $displayName ?? $identity->display_name;
                $identity->claims_json = $claims;
                $identity->save();

                $verifiedAt = $this->syncVerification($identity, $user, $provider, $providerUid, $emailVerified);

                return $this->toProvisionedUser($user, $email ?? $user->email, $verifiedAt);
            }

            // 2) identity が無ければ email で既存Userを拾う（= シーダー復活）
            $user = null;
            if (is_string($email) && trim($email) !== '') {
                $user = User::where('email', $email)->first();
            }

            // 3) いなければ新規作成
            if (!$user) {
                $user = User::create([
                    'name' => $displayName ?: 'user',
                    'email' => $email,
                    'email_verified_at' => ($emailVerified === true) ? Carbon::now() : null,
                ]);
            }

            // 4) identity 作成
            $identity = UserIdentity::create([
                'user_id' => $user->id,
                'provider' => $provider,
                'provider_uid' => $providerUid,
                'email' => $email,
                'display_name' => $displayName,
                'claims_json' => $claims,
            ]);

            $verifiedAt = $this->syncVerification($identity, $user, $provider, $providerUid, $emailVerified);

            return $this->toProvisionedUser($user, $email ?? $user->email, $verifiedAt);
        });
    }

    public function provisionFromJwt(int $userId): ProvisionedUser
    {
        $user = User::find($userId);
        if (!$user) {
            throw new AuthenticationException("user not found: {$userId}");
        }

        $verifiedAt = $user->email_verified_at ? Carbon::parse($user->email_verified_at) : null;

        return $this->toProvisionedUser($user, $user->email, $verifiedAt);
    }

    private function syncVerification(
        UserIdentity $identity,
        User $user,
        string $provider,
        string $providerUid,
        ?bool $emailVerified,
    ): ?Carbon {
        // claim が true でなければ「上書きしない」
        if ($emailVerified !== true) {
            return $user->email_verified_at ? Carbon::parse($user->email_verified_at) : null;
        }

        $now = Carbon::now();

        UserIdentityVerification::updateOrCreate(
            ['user_identity_id' => $identity->id, 'type' => 'email'],
            [
                'verified_at' => $now,
                'verified_provider' => $provider,
                'verified_subject' => $providerUid,
                'evidence_json' => ['source' => 'access_token_claims'],
            ]
        );

        // 旧互換：users 側も埋める
        if (!$user->hasVerifiedEmail()) {
            $user->forceFill(['email_verified_at' => $now])->save();
        }

        return $now;
    }

    private function toProvisionedUser(User $user, ?string $email, ?Carbon $verifiedAt): ProvisionedUser
    {
        // roles / shopIds
        $roleRows = $user->roles()->get(['roles.slug']);
        $roles = $roleRows->pluck('slug')->unique()->values()->all();
        $shopIds = $roleRows->pluck('pivot.shop_id')->filter()->unique()->values()->all();

        // email verified
        $emailVerifiedFinal = $verifiedAt !== null;

        // emailVerifiedAt は ISO8601 文字列
        $emailVerifiedAtIso = $verifiedAt ? $verifiedAt->toIso8601String() : null;

        // isFirstLogin: 列が存在する場合だけ判定（無いなら false）
        $attrs = $user->getAttributes();
        $hasFirstLoginColumn = array_key_exists('first_login_at', $attrs);
        $isFirstLogin = $hasFirstLoginColumn ? empty($user->first_login_at) : false;

        // tenantId: 列がある場合だけ
        $tenantId = array_key_exists('tenant_id', $attrs) && $user->tenant_id !== null
            ? (int) $user->tenant_id
            : null;

        return new ProvisionedUser(
            userId: (int) $user->id,
            email: $email,
            emailVerified: $emailVerifiedFinal,
            emailVerifiedAt: $emailVerifiedAtIso,
            isFirstLogin: $isFirstLogin,
            shopIds: $shopIds,
            roles: $roles,
            tenantId: $tenantId,
        );
    }
}