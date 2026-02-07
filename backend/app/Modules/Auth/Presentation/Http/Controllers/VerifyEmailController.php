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

        if (! hash_equals(sha1($user->getEmailForVerification()), $hash)) {
            abort(403, 'Invalid verification link.');
        }

        if (! $user->hasVerifiedEmail()) {
            $user->markEmailAsVerified();
            event(new Verified($user));
            Log::info('[VerifyEmail] verified', ['user_id' => $user->id]);
        }

        $frontend = rtrim((string) config('app.frontend_url'), '/');

        // ✅ ここだけ変える：profile_completed 済みなら profile 強制しない
        $returnTo = $user->profile_completed ? '/' : '/mypage/profile';

        return redirect()->to(
            $frontend . '/auth/callback?screen=verify_done&returnTo=' . urlencode($returnTo)
        );
    }
}