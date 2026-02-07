<?php

declare(strict_types=1);

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Models\User;
use App\Models\UserIdentity;
use App\Models\UserIdentityVerification;
use App\Modules\Auth\Infrastructure\External\Auth0\Auth0ManagementApiClient;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;

final class Auth0SecondVerifyCallbackController extends Controller
{
    public function __invoke(Request $request)
    {
        $auth0UserId = (string) $request->query('sub');
        if ($auth0UserId === '') abort(400, 'sub is required');

        $client = Auth0ManagementApiClient::fromConfig();
        $auth0User = $client->getUser($auth0UserId);

        $email = $auth0User['email'] ?? null;
        $name = $auth0User['name'] ?? ($auth0User['nickname'] ?? 'user');
        $emailVerified = (bool) ($auth0User['email_verified'] ?? false);

        if (!$emailVerified) abort(403, 'Auth0 email_verified is false');

        DB::transaction(function () use ($auth0UserId, $email, $name) {
            // identity を探す（無ければ作る）
            $identity = UserIdentity::query()
                ->where('provider', 'auth0')
                ->where('provider_uid', $auth0UserId)
                ->first();

            if (!$identity) {
                $user = (is_string($email) && $email !== '')
                    ? User::where('email', $email)->first()
                    : null;

                if (!$user) {
                    $user = User::create([
                        'name' => $name,
                        'email' => $email,
                        // 二段階SoTなら users.email_verified_at は触らない
                        'email_verified_at' => null,
                    ]);
                }

                $identity = UserIdentity::create([
                    'user_id' => (int) $user->id,
                    'provider' => 'auth0',
                    'provider_uid' => $auth0UserId,
                    'email' => $email,
                    'display_name' => $name,
                    'claims_json' => [],
                ]);
            } else {
                $user = User::findOrFail((int) $identity->user_id);
            }

            // ✅ 本丸：二次認証SoT
            UserIdentityVerification::updateOrCreate(
                ['user_identity_id' => (int) $identity->id, 'type' => 'email_second'],
                [
                    'verified_at' => now(),
                    'verified_provider' => 'auth0',
                    'verified_subject' => $auth0UserId,
                    'evidence_json' => [
                        'source' => 'auth0_verify_ticket_result',
                        'email' => $email,
                    ],
                ]
            );

            Log::info('[Auth0SecondVerify] saved', [
                'user_id' => (int) $user->id,
                'identity_id' => (int) $identity->id,
                'auth0_user_id' => $auth0UserId,
            ]);
        });

        $frontend = rtrim((string) config('app.frontend_url'), '/');
        // ここは好みで
        return redirect()->to($frontend . '/auth/callback?screen=verify_second_done&returnTo=%2F');
    }
}