<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Application\UseCase\RunArtifacts;

use App\Modules\AtlasKernel\Application\Contracts\RunArtifacts\V1\RunArtifactContentV1;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactQueryRepository;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\ArtifactKind;

final class ListRunArtifactsUseCase
{
    public function __construct(
        private readonly RunArtifactQueryRepository $repo,
    ) {}

    /**
     * @return array<int,array{artifact_kind:string, content:array<string,mixed>, created_at:string, updated_at:string}>
     */
    public function handle(string $runId, string $traceId): array
    {
        $rows = $this->repo->listByRunId($runId);

        $out = [];
        foreach ($rows as $r) {
            $kind = $r['artifact_kind'];
            ArtifactKind::assertValid($kind);

            $dto = RunArtifactContentV1::fromArray($r['content_json']);
            $dto->validateOrThrow(
                artifactKind: $kind,
                runId: $runId,
                traceId: $traceId,
            );

            $out[] = [
                'artifact_kind' => $kind,
                'content' => $dto->toArray(),
                'created_at' => $r['created_at'],
                'updated_at' => $r['updated_at'],
            ];
        }

        return $out;
    }
}