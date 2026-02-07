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
                    $this->ensureLegacyIdentityAndSecondVerification($user);
                }
            });
    }

    private function ensureLegacyIdentityAndSecondVerification(User $user): void
    {
        $provider = 'legacy';
        $providerUid = 'user:' . (string) $user->id;

        // 1) identity を最低保証
        $identity = UserIdentity::query()->updateOrCreate(
            [
                'provider' => $provider,
                'provider_uid' => $providerUid,
            ],
            [
                'user_id' => (int) $user->id,
                'email' => $user->email,
                'display_name' => $user->name ?? null,
                'claims_json' => [
                    'source' => 'seed_backfill',
                    'note' => 'legacy identity for seeded users',
                ],
            ]
        );

        // 2) verifiedAt の決定（旧仕様から移植）
        $verifiedAt = null;

        if (method_exists($user, 'hasVerifiedEmail')) {
            if ($user->hasVerifiedEmail()) {
                $verifiedAt = $user->email_verified_at ?? now();
            }
        } else {
            $verifiedAt = $user->email_verified_at ?? null;
        }

        // 3) ✅ /api/me のSoTに合わせて email_second を立てる
        if ($verifiedAt) {
            UserIdentityVerification::query()->updateOrCreate(
                [
                    'user_identity_id' => (int) $identity->id,
                    'type' => 'email_second',
                ],
                [
                    'verified_at' => $verifiedAt,
                    'verified_provider' => 'legacy',
                    'verified_subject' => $providerUid,
                    'evidence_json' => [
                        'copied_from' => 'users.email_verified_at',
                        'copied_at' => now()->toIso8601String(),
                    ],
                ]
            );
        } else {
            // 未検証ユーザーは email_second を作らない（= 403 のまま）
            // 作るなら verified_at null になるので /api/me では無効、存在させる意味が薄い
        }
    }
}
