<?php

declare(strict_types=1);

namespace App\Modules\Item\Presentation\Http\Controllers\AtlasKernel;

use App\Http\Controllers\Controller;
use App\Models\User;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

final class AdminAtlasRequestController extends Controller
{
    public function __construct(
        private AtlasRequestController $atlasRequestController,
    ) {
    }

    public function index(Request $request): JsonResponse
    {
        $user = $request->user();

        if (! $user instanceof User) {
            return response()->json([
                'message' => 'Unauthenticated',
            ], Response::HTTP_UNAUTHORIZED);
        }

        if (! $this->hasGlobalAtlasAdminRole($user)) {
            return response()->json([
                'message' => 'Forbidden',
                'code' => 'atlas_admin_forbidden',
            ], Response::HTTP_FORBIDDEN);
        }

        /**
         * Phase 1 暫定方針:
         * - 既存の shop 一覧ロジックを極力流用したい
         * - ただし ALL は実在 shop ではないため、shop.context 前提は使わない
         * - ここでは Admin 向け Query 実装へ切り出すのが本来だが、
         *   まだ無い場合は AtlasRequestController 内の一覧取得ロジックを
         *   service / query に寄せて共用するのが次の一手
         *
         * まずは専用メソッドを AtlasRequestController 側に追加して使う。
         */
        return $this->atlasRequestController->adminIndex($request);
    }

    private function hasGlobalAtlasAdminRole(User $user): bool
    {
        return $user->roles()
            ->wherePivotNull('shop_id')
            ->whereIn('slug', ['domain_lead_admin'])
            ->exists();
    }
}