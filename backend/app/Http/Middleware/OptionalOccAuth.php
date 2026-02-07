<?php

namespace App\Http\Middleware;

use App\Modules\Auth\Application\Context\AuthContext;
use App\Modules\Auth\Application\Service\JwtUserResolver;
use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use Closure;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;

final class OptionalOccAuth
{
    public function __construct(
        private JwtUserResolver $jwt,
        private AuthContext $authContext,
    ) {}

    public function handle(Request $request, Closure $next)
    {
        $this->authContext->clear();

        // 1) JWT
        $resolved = $this->jwt->resolve($request);
        if ($resolved) {
            Auth::setUser($resolved['user']);
            $request->setUserResolver(fn () => $resolved['user']);

            $principal = $resolved['principal'];
            $this->authContext->setPrincipal($principal);

            // ✅ 追加
            $request->attributes->set('auth_principal', $principal);

            return $next($request);
        }

        // 2) ✅ web（stateful本命）
        $user = Auth::guard('web')->user();
        if ($user) {
            Auth::setUser($user);
            $request->setUserResolver(fn () => $user);

            $principal = AuthPrincipal::fromSanctumUser($user);
            $this->authContext->setPrincipal($principal);

            // ✅ 追加
            $request->attributes->set('auth_principal', $principal);

            return $next($request);
        }

        // 3) optional fallback: sanctum
        $user = Auth::guard('sanctum')->user();
        if ($user) {
            Auth::setUser($user);
            $request->setUserResolver(fn () => $user);

            $principal = AuthPrincipal::fromSanctumUser($user);
            $this->authContext->setPrincipal($principal);

            // ✅ 追加
            $request->attributes->set('auth_principal', $principal);
        }

        return $next($request);
    }
}