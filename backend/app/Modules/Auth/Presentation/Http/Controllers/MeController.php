<?php

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Auth\Application\Context\AuthContext;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;

final class MeController extends Controller
{
    public function __construct(
        private AuthContext $authContext,
    ) {
    }

    public function __invoke(Request $request): JsonResponse
    {
        $user = $request->user();
        $principal = $this->authContext->principal();

        if (! $user || ! $principal) {
            return response()->json(['message' => 'Unauthenticated'], 401);
        }

        $roles = $user->roles()
            ->wherePivotNotNull('shop_id')
            ->get(['roles.slug']);

        $shopIds = $roles->pluck('pivot.shop_id')->unique()->values();

        $shopCodeById = \App\Models\Shop::query()
            ->whereIn('id', $shopIds)
            ->pluck('shop_code', 'id');

        $shopRoles = $roles->map(fn ($role) => [
            'shop_id'   => (int) $role->pivot->shop_id,
            'shop_code' => $shopCodeById[$role->pivot->shop_id] ?? null,
            'role'      => $role->slug,
        ])->values();

        return response()->json([
            'id' => $user->id,

            // ✅ principal を真実にする（分離SoTに同期）
            'email' => $principal->email(),
            'display_name' => $user->name,

            // ✅ verifiedは principal に従う
            // ※ ここで verified_at の厳密な時刻を返したいなら principal に verifiedAt を持たせる（次段階）
            'email_verified_at' => $principal->emailVerifiedAt(),

            'profile_completed' => $user->profile_completed,
            'shop_roles' => $shopRoles,
        ]);
    }
}