<?php

declare(strict_types=1);

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
            $provider,
            $providerUid,
            $email,
            $emailVerified,
            $displayName,
            $claims
        ) {
            // 1) provider+uid で identity を探す
            $identity = UserIdentity::query()
                ->where('provider', $provider)
                ->where('provider_uid', $providerUid)
                ->first();

            if ($identity) {
                $user = User::query()->find($identity->user_id);
                if (! $user) {
                    throw new \RuntimeException("user not found for identity: {$identity->id}");
                }

                // 任意：identity を最新化
                $identity->email = $email ?? $identity->email;
                $identity->display_name = $displayName ?? $identity->display_name;
                $identity->claims_json = $claims;
                $identity->save();

                // ✅ IdP 側 verified は「email_provider」にだけ同期（SoTはemail_second）
                $this->syncProviderVerification(
                    identity: $identity,
                    provider: $provider,
                    providerUid: $providerUid,
                    emailVerified: $emailVerified,
                );

                // ✅ ProvisionedUser の verified 判定は常に email_second
                return $this->toProvisionedUser($user, $identity, $email ?? $user->email);
            }

            // 2) identity が無ければ email で既存Userを拾う（シーダー復活）
            $user = null;
            if (is_string($email) && trim($email) !== '') {
                $user = User::query()->where('email', $email)->first();
            }

            // 3) いなければ新規作成（❌ users.email_verified_at は触らない）
            if (! $user) {
                $user = User::query()->create([
                    'name' => $displayName ?: 'user',
                    'email' => $email,
                    'email_verified_at' => null, // ✅ 二段階運用では SoT をここに置かない
                ]);
            } else {
                // 任意：表示名やメールを更新したいならここで
                // ただし email 変更は要件次第なので、勝手に上書きはしない方が安全
            }

            // 4) identity 作成
            $identity = UserIdentity::query()->create([
                'user_id' => $user->id,
                'provider' => $provider,
                'provider_uid' => $providerUid,
                'email' => $email,
                'display_name' => $displayName,
                'claims_json' => $claims,
            ]);

            // ✅ IdP 側 verified は「email_provider」へ
            $this->syncProviderVerification(
                identity: $identity,
                provider: $provider,
                providerUid: $providerUid,
                emailVerified: $emailVerified,
            );

            // ✅ ProvisionedUser の verified 判定は常に email_second
            return $this->toProvisionedUser($user, $identity, $email ?? $user->email);
        });
    }

    public function provisionFromJwt(int $userId): ProvisionedUser
    {
        $user = User::query()->find($userId);
        if (! $user) {
            throw new AuthenticationException("user not found: {$userId}");
        }

        // JWT方式でも「二次認証SoT(email_second)」を見に行く
        $identity = UserIdentity::query()
            ->where('user_id', (int) $user->id)
            ->orderByDesc('id')
            ->first();

        // identity が無いなら「二次未認証扱い」（= verified false）
        if (! $identity) {
            return $this->toProvisionedUser($user, null, $user->email);
        }

        return $this->toProvisionedUser($user, $identity, $user->email);
    }

    /**
     * IdP側の email_verified を記録する（SoTは email_second ではない）
     */
    private function syncProviderVerification(
        UserIdentity $identity,
        string $provider,
        string $providerUid,
        ?bool $emailVerified,
    ): void {
        if ($emailVerified !== true) {
            return;
        }

        $now = Carbon::now();

        UserIdentityVerification::query()->updateOrCreate(
            ['user_identity_id' => $identity->id, 'type' => 'email_provider'],
            [
                'verified_at' => $now,
                'verified_provider' => $provider,
                'verified_subject' => $providerUid,
                'evidence_json' => ['source' => 'access_token_claims'],
            ]
        );

        // ❌ 二段階にするなら users.email_verified_at をここで触らない
    }

    /**
     * ProvisionedUser を「二次認証SoT(email_second)」基準で構築する
     */
    private function toProvisionedUser(User $user, ?UserIdentity $identity, ?string $email): ProvisionedUser
    {
        // roles / shopIds
        $roleRows = $user->roles()->get(['roles.slug']);
        $roles = $roleRows->pluck('slug')->unique()->values()->all();
        $shopIds = $roleRows->pluck('pivot.shop_id')->filter()->unique()->values()->all();

        // ✅ 二次認証(SoT)の verified_at を取る
        $secondVerifiedAt = $this->getSecondVerifiedAt($user, $identity);

        $emailVerifiedFinal = $secondVerifiedAt !== null;
        $emailVerifiedAtIso = $secondVerifiedAt?->toIso8601String();

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

    /**
     * 二次メール認証 SoT(email_second) を取得
     * - identity が渡されればその identity に紐づく email_second を優先
     * - 無ければ user_id で join して拾う（複数identity対策）
     */
    private function getSecondVerifiedAt(User $user, ?UserIdentity $identity): ?Carbon
    {
        // 1) identity があるならまずそれに紐づく email_second を見る（最優先）
        if ($identity) {
            $v = UserIdentityVerification::query()
                ->where('user_identity_id', (int) $identity->id)
                ->where('type', 'email_second')
                ->orderByDesc('verified_at')
                ->value('verified_at');

            if ($v) {
                return Carbon::parse($v);
            }
        }

        // 2) 保険：user_id で join して拾う（identity が複数あるケース）
        $v = DB::table('user_identity_verifications as v')
            ->join('user_identities as i', 'i.id', '=', 'v.user_identity_id')
            ->where('i.user_id', (int) $user->id)
            ->where('v.type', 'email_second')
            ->orderByDesc('v.verified_at')
            ->value('v.verified_at');

        return $v ? Carbon::parse((string) $v) : null;
    }
}