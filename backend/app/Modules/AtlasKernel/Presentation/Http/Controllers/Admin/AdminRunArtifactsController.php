<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Presentation\Http\Controllers\Admin;

use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;
use App\Modules\AtlasKernel\Application\UseCase\RunArtifacts\Admin\ListAdminRunArtifactsUseCase;

final class AdminRunArtifactsController
{
    public function __construct(
        private readonly ListAdminRunArtifactsUseCase $uc,
    ) {}

    public function index(Request $req): JsonResponse
    {
        $kind   = $req->query('kind');
        $q      = $req->query('q');
        $limit  = (int)$req->query('limit', 50);
        $cursor = $req->query('cursor');

        $data = $this->uc->handle(
            kind: is_string($kind) ? $kind : null,
            q: is_string($q) ? $q : null,
            limit: $limit,
            cursor: is_string($cursor) ? $cursor : null
        );

        return response()->json($data, 200);
    }
}