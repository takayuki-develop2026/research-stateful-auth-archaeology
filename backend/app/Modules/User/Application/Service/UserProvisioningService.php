<?php

namespace App\Modules\User\Application\Service;

use App\Models\User;
use Illuminate\Support\Facades\DB;
use App\Modules\Auth\Domain\Port\UserProvisioningPort;
use App\Modules\Auth\Domain\Dto\ProvisionedUser;
use App\Modules\User\Domain\Repository\ProfileRepository;
use App\Modules\User\Domain\Entity\Profile;

final class UserProvisioningService implements UserProvisioningPort
{
    public function __construct(
        private ProfileRepository $profiles,
    ) {
    }

    public function provisionFromFirebase(
        string $firebaseUid,
        ?string $email,
        bool $emailVerified,
        ?string $displayName,
    ): ProvisionedUser {
        if (! $email) {
            throw new \DomainException('Email is required for user provisioning.');
        }

        return DB::transaction(function () use ($firebaseUid, $email, $emailVerified, $displayName) {

            $user = User::where('firebase_uid', $firebaseUid)->first()
                ?? User::where('email', $email)->first();

            $isFirstLogin = false;

            if (! $user) {
                $user = User::create([
                    'firebase_uid'      => $firebaseUid,
                    'name'              => $displayName ?? 'User',
                    'email'             => $email,
                    'email_verified_at' => null, // ✅ 分離SoT
                    'first_login_at'    => now(),
                ]);
                $isFirstLogin = true;
            } else {
                $updates = [];

                if (! $user->firebase_uid) {
                    $updates['firebase_uid'] = $firebaseUid;
                } elseif ($user->firebase_uid !== $firebaseUid) {
                    throw new \DomainException('Firebase UID mismatch.');
                }

                if (! $user->first_login_at) {
                    $updates['first_login_at'] = now();
                }

                if ($updates) {
                    $user->update($updates);
                }
            }

            $identityId = $this->upsertUserIdentity(
                userId: $user->id,
                provider: 'firebase',
                providerUid: $firebaseUid,
                email: $email,
                displayName: $displayName,
                claims: [],
            );

            // ✅ verified true の時だけ verified_at をセット（falseでは消さない）
            if ($emailVerified === true) {
                $this->upsertEmailVerificationVerified(
                    identityId: $identityId,
                    verifiedProvider: 'firebase',
                    verifiedSubject: $firebaseUid,
                    evidence: [],
                );
            }

            $this->ensureProfileExists($user->id, $displayName ?? $user->name);

            return $this->buildProvisionedUserFromIdentityId(
                userId: $user->id,
                identityId: $identityId,
                isFirstLogin: $isFirstLogin
            );
        });
    }

    public function provisionFromExternalIdentity(
        string $provider,
        string $providerUid,
        ?string $email = null,
        ?bool $emailVerified = null,
        ?string $displayName = null,
        array $claims = [],
    ): ProvisionedUser {
        return DB::transaction(function () use ($provider, $providerUid, $email, $emailVerified, $displayName, $claims) {

            $identity = DB::table('user_identities')
                ->where('provider', $provider)
                ->where('provider_uid', $providerUid)
                ->first();

            $user = null;
            $isFirstLogin = false;

            if ($identity) {
                $user = User::find($identity->user_id);
            }

            // ✅ Emailでの既存User探索は「emailVerified === true」のときのみ
if (! $user && $email && $emailVerified === true) {
    $user = User::where('email', $email)->first();
}

            if (! $user) {
                if (! $email) {
                    throw new \DomainException('Email is required for external identity provisioning.');
                }

                $user = User::create([
                    'name'              => $displayName ?? 'User',
                    'email'             => $email,
                    'email_verified_at' => null, // ✅ 分離SoT
                    'first_login_at'    => now(),
                ]);
                $isFirstLogin = true;
            } else {
                if (! $user->first_login_at) {
                    $user->update(['first_login_at' => now()]);
                }
            }

            $identityId = $this->upsertUserIdentity(
                userId: $user->id,
                provider: $provider,
                providerUid: $providerUid,
                email: $email,
                displayName: $displayName,
                claims: $claims,
            );

            // ✅ null: 不明 -> 触らない
            // ✅ false: “未認証” -> 既存verifiedを消さない（安全側）
            // ✅ true: verified_at をセット
            if ($emailVerified === true) {
                $this->upsertEmailVerificationVerified(
                    identityId: $identityId,
                    verifiedProvider: $provider,
                    verifiedSubject: $providerUid,
                    evidence: $claims,
                );
            }

            $this->ensureProfileExists($user->id, $displayName ?? $user->name);

            return $this->buildProvisionedUserFromIdentityId(
                userId: $user->id,
                identityId: $identityId,
                isFirstLogin: $isFirstLogin
            );
        });
    }

    public function provisionFromJwt(int $userId): ProvisionedUser
    {
        return DB::transaction(function () use ($userId) {
            $user = User::find($userId);

            if (! $user) {
                throw new \DomainException('User not found for JWT provisioning.');
            }

            // ✅ 互換: provider不明なので users.email_verified_at を参照（legacyのみ）
            return $this->buildProvisionedUserLegacyUserTable($user->id, false);
        });
    }

    public function provisionFromAuth0(
        string $auth0Sub,
        ?string $email,
        bool $emailVerified,
        ?string $displayName,
        array $claims = [],
    ): ProvisionedUser {
        return $this->provisionFromExternalIdentity(
            provider: 'auth0',
            providerUid: $auth0Sub,
            email: $email,
            emailVerified: $emailVerified,
            displayName: $displayName,
            claims: $claims,
        );
    }

    /* =========================================================
       Internal helpers
    ========================================================= */

    private function ensureProfileExists(int $userId, string $displayName): void
    {
        $profile = $this->profiles->findByUserId($userId);

        if (! $profile) {
            $this->profiles->save(Profile::createEmpty(
                userId: $userId,
                displayName: $displayName
            ));
        }
    }

    /**
     * ✅ 分離SoTから確定する（identityId で一意に引く）
     */
    private function buildProvisionedUserFromIdentityId(
        int $userId,
        int $identityId,
        bool $isFirstLogin
    ): ProvisionedUser {
        $user = User::find($userId);

        $identity = DB::table('user_identities')->where('id', $identityId)->first();
        $email = $identity?->email ?? $user?->email;

        $ver = DB::table('user_identity_verifications')
            ->where('user_identity_id', $identityId)
            ->where('type', 'email')
            ->first();

        $emailVerifiedAt = $ver?->verified_at ? (string) $ver->verified_at : null;
        $emailVerified = $emailVerifiedAt !== null;

        $shopIds = DB::table('role_user')
            ->where('user_id', $userId)
            ->pluck('shop_id')
            ->filter()
            ->values()
            ->all();

        // ✅ roles は slug に統一（AuthPrincipal と一致させる）
        $roles = DB::table('role_user')
            ->join('roles', 'roles.id', '=', 'role_user.role_id')
            ->where('role_user.user_id', $userId)
            ->pluck('roles.slug')
            ->values()
            ->all();

        return new ProvisionedUser(
            userId: $userId,
            email: is_string($email) ? $email : null,
            emailVerified: $emailVerified,
            emailVerifiedAt: $emailVerifiedAt,
            isFirstLogin: $isFirstLogin,
            shopIds: $shopIds,
            roles: $roles,
            tenantId: $shopIds[0] ?? null,
        );
    }

    /**
     * ✅ 互換用（sub=内部user_id の時だけ）
     */
    private function buildProvisionedUserLegacyUserTable(int $userId, bool $isFirstLogin): ProvisionedUser
    {
        $user = User::find($userId);

        $shopIds = DB::table('role_user')
            ->where('user_id', $userId)
            ->pluck('shop_id')
            ->filter()
            ->values()
            ->all();

        $roles = DB::table('role_user')
            ->join('roles', 'roles.id', '=', 'role_user.role_id')
            ->where('role_user.user_id', $userId)
            ->pluck('roles.slug')
            ->values()
            ->all();

        return new ProvisionedUser(
            userId: $userId,
            email: $user?->email,
            emailVerified: (bool) ($user?->email_verified_at),
            emailVerifiedAt: $user?->email_verified_at ? $user->email_verified_at->toISOString() : null,
            isFirstLogin: $isFirstLogin,
            shopIds: $shopIds,
            roles: $roles,
            tenantId: $shopIds[0] ?? null,
        );
    }

    /**
     * ✅ identity link upsert（id を返す）
     */
    private function upsertUserIdentity(
        int $userId,
        string $provider,
        string $providerUid,
        ?string $email,
        ?string $displayName,
        array $claims,
    ): int {
        $now = now();

        $existing = DB::table('user_identities')
            ->where('provider', $provider)
            ->where('provider_uid', $providerUid)
            ->first();

        if ($existing) {
            DB::table('user_identities')
                ->where('id', $existing->id)
                ->update([
                    'user_id' => $userId,
                    'email' => $email,
                    'display_name' => $displayName,
                    'claims_json' => $claims ? json_encode($claims, JSON_UNESCAPED_UNICODE) : null,
                    'updated_at' => $now,
                ]);

            return (int) $existing->id;
        }

        $id = DB::table('user_identities')->insertGetId([
            'user_id' => $userId,
            'provider' => $provider,
            'provider_uid' => $providerUid,
            'email' => $email,
            'display_name' => $displayName,
            'claims_json' => $claims ? json_encode($claims, JSON_UNESCAPED_UNICODE) : null,
            'created_at' => $now,
            'updated_at' => $now,
        ]);

        return (int) $id;
    }

    /**
     * ✅ verified SoT（安全側：verified=true の時だけ verified_at をセット）
     */
    private function upsertEmailVerificationVerified(
        int $identityId,
        ?string $verifiedProvider,
        ?string $verifiedSubject,
        array $evidence
    ): void {
        $now = now();

        $existing = DB::table('user_identity_verifications')
            ->where('user_identity_id', $identityId)
            ->where('type', 'email')
            ->first();

        if ($existing) {
            DB::table('user_identity_verifications')
                ->where('id', $existing->id)
                ->update([
                    'verified_at' => $existing->verified_at ?: $now, // ✅ 既にあるなら保持
                    'verified_provider' => $verifiedProvider,
                    'verified_subject' => $verifiedSubject,
                    'evidence_json' => $evidence ? json_encode($evidence, JSON_UNESCAPED_UNICODE) : null,
                    'updated_at' => $now,
                ]);
            return;
        }

        DB::table('user_identity_verifications')->insert([
            'user_identity_id' => $identityId,
            'type' => 'email',
            'verified_at' => $now,
            'verified_provider' => $verifiedProvider,
            'verified_subject' => $verifiedSubject,
            'evidence_json' => $evidence ? json_encode($evidence, JSON_UNESCAPED_UNICODE) : null,
            'created_at' => $now,
            'updated_at' => $now,
        ]);
    }
}