<?php

declare(strict_types=1);

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Auth\Application\UseCase\SendEmailVerificationTicketUseCase;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use Illuminate\Http\Request;

final class SendEmailVerificationTicketController extends Controller
{
    public function __construct(
        private readonly SendEmailVerificationTicketUseCase $useCase,
    ) {
    }

    public function __invoke(Request $request)
    {
        $principal = $this->resolvePrincipal($request);

        if (!$principal instanceof AuthPrincipal) {
            return response()->json(['error' => 'AuthPrincipal missing'], 401);
        }

        $this->useCase->handle($principal);

        return response()->json(['ok' => true], 200);
    }

    private function resolvePrincipal(Request $request): ?AuthPrincipal
    {
        // 1) あなたの実装が request attributes に載せている場合（最も一般的）
        $p = $request->attributes->get('auth_principal');
        if ($p instanceof AuthPrincipal) {
            return $p;
        }

        // 2) AuthContext がDI/コンテナにいるならそこから（存在しても依存しないように string 参照）
        $authContextClass = '\\App\\Modules\\Auth\\Application\\Dto\\AuthContext';
        if (class_exists($authContextClass) && app()->bound($authContextClass)) {
            $ctx = app($authContextClass);
            if (is_object($ctx) && method_exists($ctx, 'principal')) {
                $pp = $ctx->principal();
                if ($pp instanceof AuthPrincipal) {
                    return $pp;
                }
            }
        }

        // 3) 最後の保険：Laravel user に provider_uid 等がある場合（無ければ null）
        $u = $request->user();
        if (is_object($u)) {
            $provider = $u->provider ?? null;
            $providerUid = $u->provider_uid ?? ($u->auth0_sub ?? ($u->external_identity_sub ?? null));
            $email = $u->email ?? null;
            $emailVerified = (bool) ($u->email_verified_at ?? false);
            $emailVerifiedAt = null;

            try {
                if (isset($u->email_verified_at) && $u->email_verified_at) {
                    $emailVerifiedAt = method_exists($u->email_verified_at, 'toISOString')
                        ? $u->email_verified_at->toISOString()
                        : (string) $u->email_verified_at;
                }
            } catch (\Throwable) {
                $emailVerifiedAt = null;
            }

            if (is_string($provider) && is_string($providerUid) && $provider !== '' && $providerUid !== '') {
                return AuthPrincipal::fromJwtPayload(
                    userId: (int) ($u->id ?? 0),
                    email: is_string($email) ? $email : null,
                    provider: $provider,
                    providerUid: $providerUid,
                    emailVerified: $emailVerified,
                    emailVerifiedAt: is_string($emailVerifiedAt) ? $emailVerifiedAt : null,
                    roles: [],
                    shopRoles: [],
                );
            }
        }

        return null;
    }
}