<?php

declare(strict_types=1);

namespace Database\Seeders;

use App\Models\User;
use App\Models\UserIdentity;
use App\Models\UserIdentityVerification;
use Illuminate\Database\Seeder;

final class BackfillLegacyIdentitySeeder extends Seeder
{
    public function run(): void
    {
        User::query()
            ->orderBy('id')
            ->chunkById(200, function ($users) {
                foreach ($users as $user) {
                    $this->ensureLegacyIdentity($user);
                }
            });
    }

    private function ensureLegacyIdentity(User $user): void
    {
        // 旧ユーザー用の “最低保証 identity”
        // provider/provider_uid は unique(provider, provider_uid) を満たす必要がある
        $provider = 'legacy';
        $providerUid = 'user:' . (string) $user->id;

        $identity = UserIdentity::query()->updateOrCreate(
            [
                'provider' => $provider,
                'provider_uid' => $providerUid,
            ],
            [
                'user_id' => $user->id,
                'email' => $user->email,
                'display_name' => $user->name ?? null,
                'claims_json' => [
                    'source' => 'seed_backfill',
                    'note' => 'legacy identity for seeded users',
                ],
            ]
        );

        // 旧仕様の email verification を “新仕様”へ移植
        // mustVerifyEmail を使っているなら hasVerifiedEmail() を優先
        $verifiedAt = null;

        if (method_exists($user, 'hasVerifiedEmail')) {
            if ($user->hasVerifiedEmail()) {
                $verifiedAt = $user->email_verified_at ?? now();
            }
        } else {
            $verifiedAt = $user->email_verified_at ?? null;
        }

        UserIdentityVerification::query()->updateOrCreate(
            [
                'user_identity_id' => $identity->id,
                'type' => 'email',
            ],
            [
                'verified_at' => $verifiedAt,
                'verified_provider' => $verifiedAt ? 'legacy' : null,
                'verified_subject' => $verifiedAt ? $providerUid : null,
                'evidence_json' => $verifiedAt ? [
                    'copied_from' => 'users.email_verified_at',
                    'copied_at' => now()->toIso8601String(),
                ] : null,
            ]
        );
    }
}