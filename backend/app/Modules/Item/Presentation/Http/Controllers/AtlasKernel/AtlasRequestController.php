<?php

declare(strict_types=1);

namespace App\Modules\Item\Presentation\Http\Controllers\AtlasKernel;

use App\Http\Controllers\Controller;
use App\Modules\Item\Application\Assembler\AtlasKernel\AtlasRequestListAssembler;
use App\Modules\Item\Application\Query\AtlasRequestListQuery;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

final class AtlasRequestController extends Controller
{
    public function __construct(
        private AtlasRequestListQuery $query,
        private AtlasRequestListAssembler $assembler,
    ) {
    }

    /**
     * Shop scope 一覧
     * GET /api/shops/{shop_code}/atlas/requests
     */
    public function index(string $shop_code): JsonResponse
    {
        $rows = $this->query->listByShopCode($shop_code);
        $requests = $this->assembler->assembleMany($rows);

        return response()->json([
            'requests' => $requests,
        ]);
    }

    /**
     * Admin scope 一覧（Phase 1 暫定）
     * GET /api/admin/atlas/requests
     *
     * - global role を持つユーザーだけが全 shop 一覧を見られる
     * - フロントの /shops/ALL/dashboard/atlas/requests から使用
     */
    public function adminIndex(Request $request): JsonResponse
    {
        $user = $request->user();

        if (! $user) {
            return response()->json([
                'message' => 'Unauthenticated',
            ], Response::HTTP_UNAUTHORIZED);
        }

        $isGlobalAtlasAdmin = $user->roles()
            ->wherePivotNull('shop_id')
            ->whereIn('slug', ['domain_lead_admin'])
            ->exists();

        if (! $isGlobalAtlasAdmin) {
            return response()->json([
                'message' => 'Forbidden',
                'code' => 'atlas_admin_forbidden',
            ], Response::HTTP_FORBIDDEN);
        }

        $rows = $this->query->listAllShops();
        $requests = $this->assembler->assembleMany($rows);

        return response()->json([
            'requests' => $requests,
        ]);
    }
}