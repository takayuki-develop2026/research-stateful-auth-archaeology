<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts;

interface RunArtifactQueryRepository
{
    /**
     * @return array<string,mixed>|null  content_json decoded (associative)
     */
    public function findContentByRunIdAndKind(string $runId, string $artifactKind): ?array;

    /**
     * @return array<int,array{artifact_kind:string, content_json:array<string,mixed>, created_at:string, updated_at:string}>
     */
    public function listByRunId(string $runId): array;
}