<?php

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Models\User;
use Illuminate\Auth\Events\Verified;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Log;

final class VerifyEmailController extends Controller
{
    public function __invoke(Request $request, int $id, string $hash)
    {
        $user = User::findOrFail($id);

        // ハッシュ検証（重要）
        if (! hash_equals(sha1($user->getEmailForVerification()), $hash)) {
            abort(403, 'Invalid verification link.');
        }

        if (! $user->hasVerifiedEmail()) {
            $user->markEmailAsVerified();
            event(new Verified($user));

            Log::info('[VerifyEmail] verified', ['user_id' => $user->id]);
        }

        // ✅ ここが本丸：verify後は Next の「Auth0ログイン開始」へ飛ばす
        // そこで Auth0 authorize → /auth/callback → returnTo へ確実に戻す
        $frontend = rtrim((string) config('app.frontend_url'), '/');
        $returnTo = '/mypage/profile';

        return redirect()->away(
            $frontend . '/auth/login?returnTo=' . urlencode($returnTo)
        );
    }
}