<?php

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Models\User;
use App\Models\UserIdentity;
use App\Models\UserIdentityVerification;
use Illuminate\Auth\Events\Verified;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;

final class VerifyEmailController extends Controller
{
    public function __invoke(Request $request, int $id, string $hash)
    {
        $user = User::findOrFail($id);

        if (! hash_equals(sha1($user->getEmailForVerification()), $hash)) {
            abort(403, 'Invalid verification link.');
        }

        // ✅ どのルートで呼ばれたかで一次/二次を判定
        $routeName = (string) optional($request->route())->getName();
        $isSecond = ($routeName === 'verification.verify_second') || ($request->query('second') === '1');

        // =========================================================
        // 1) 一次認証（Laravel標準）
        // =========================================================
        if (! $user->hasVerifiedEmail()) {
            $user->markEmailAsVerified();
            event(new Verified($user));
            Log::info('[VerifyEmail] verified(primary)', ['user_id' => $user->id]);
        }

        // =========================================================
        // 2) 二次認証SoT（email_second）
        //    ✅ 二次リンクの時だけ立てる（一次リンクでは絶対に立てない）
        // =========================================================
        if ($isSecond) {
            $identity = $this->resolveOrCreateIdentityForSecondVerification($user);

            UserIdentityVerification::updateOrCreate(
                [
                    'user_identity_id' => (int) $identity->id,
                    'type' => 'email_second',
                ],
                [
                    'verified_at' => now(),
                    'verified_provider' => 'internal',
                    'verified_subject' => (string) $user->id,
                    'evidence_json' => [
                        'source' => 'second_verify_email_link',
                        'email' => $user->email,
                    ],
                ]
            );

            Log::info('[VerifyEmail] verified(second)', [
                'user_id' => $user->id,
                'identity_id' => $identity->id,
            ]);
        } else {
            Log::info('[VerifyEmail] primary_only', ['user_id' => $user->id]);
        }

        // =========================================================
        // 3) フロントへ戻す
        // =========================================================
        $frontend = rtrim((string) config('app.frontend_url'), '/');

        // 二次未完了なら second verify 画面へ誘導するのが自然（任意）
        $returnTo = $isSecond
            ? ($user->profile_completed ? '/' : '/mypage/profile')
            : '/email/verify-second'; // ← 二次を踏ませる導線（任意）

        return redirect()->to(
            $frontend . '/auth/callback?screen=verify_done&returnTo=' . urlencode($returnTo)
        );
    }

    private function resolveOrCreateIdentityForSecondVerification(User $user): UserIdentity
    {
        $identity = UserIdentity::query()
            ->where('user_id', (int) $user->id)
            ->when(
                is_string($user->email) && $user->email !== '',
                fn ($q) => $q->where('email', $user->email),
                fn ($q) => $q
            )
            ->orderByDesc('id')
            ->first();

        if ($identity) return $identity;

        // ✅ 二次SoTが必要なのに identity が無いユーザー向け：internal を作る
        $created = UserIdentity::create([
            'user_id' => (int) $user->id,
            'provider' => 'internal',
            'provider_uid' => 'user:' . (string) $user->id,
            'email' => $user->email,
            'display_name' => $user->name,
            'claims_json' => [],
        ]);

        Log::info('[VerifyEmail] identity_created_for_second', [
            'user_id' => $user->id,
            'identity_id' => $created->id,
        ]);

        return $created;
    }
}