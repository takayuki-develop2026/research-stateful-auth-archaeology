<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Application\UseCase\RunArtifacts;

use App\Modules\AtlasKernel\Application\Contracts\RunArtifacts\V1\RunArtifactContentV1;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactQueryRepository;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\ArtifactKind;

final class GetRunArtifactUseCase
{
    public function __construct(
        private readonly RunArtifactQueryRepository $repo,
    ) {}

    /** @return array<string,mixed>|null */
    public function handle(string $runId, string $traceId, string $artifactKind): ?array
    {
        ArtifactKind::assertValid($artifactKind);

        $json = $this->repo->findContentByRunIdAndKind($runId, $artifactKind);
        if ($json === null) {
            return null;
        }

        $dto = RunArtifactContentV1::fromArray($json);
        $dto->validateOrThrow(
            artifactKind: $artifactKind,
            runId: $runId,
            traceId: $traceId,
        );

        return $dto->toArray();
    }
}