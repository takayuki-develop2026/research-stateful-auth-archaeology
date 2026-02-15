<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Presentation\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Http\JsonResponse;
use App\Modules\AtlasKernel\Application\UseCase\RunArtifacts\GetRunArtifactUseCase;
use App\Modules\AtlasKernel\Application\UseCase\RunArtifacts\ListRunArtifactsUseCase;

final class RunArtifactsController
{
    public function index(Request $req, string $runId): JsonResponse
{
    $traceId = (string)$req->query('trace_id', '');
    if ($traceId === '') {
        return response()->json(['error' => 'trace_id is required'], 422);
    }

    $uc = app(ListRunArtifactsUseCase::class);
    $items = $uc->handle($runId, $traceId);

    // content -> content_json に寄せる
    $items = array_map(fn($it) => [
        'artifact_kind' => $it['artifact_kind'],
        'content_json'  => $it['content'],
        'created_at'    => $it['created_at'],
        'updated_at'    => $it['updated_at'],
    ], $items);

    return response()->json(['run_id' => $runId, 'items' => $items], 200);
}

public function show(Request $req, string $runId, string $artifactKind): JsonResponse
{
    $traceId = (string)$req->query('trace_id', '');
    if ($traceId === '') {
        return response()->json(['error' => 'trace_id is required'], 422);
    }

    $uc = app(GetRunArtifactUseCase::class);
    $content = $uc->handle($runId, $traceId, $artifactKind);

    if ($content === null) {
        return response()->json(['error' => 'not_found'], 404);
    }

    return response()->json([
        'run_id' => $runId,
        'artifact_kind' => $artifactKind,
        'content_json' => $content,
    ], 200);
}
}