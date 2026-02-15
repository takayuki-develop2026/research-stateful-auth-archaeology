<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts;

interface RunArtifactAdminQueryRepository
{
    /**
     * @param array{
     *   kind?: string|null,
     *   q?: string|null,
     *   limit?: int|null,
     *   cursor?: string|null
     * } $params
     *
     * @return array{
     *   items: array<int,array{
     *     id:int,
     *     run_id:string,
     *     trace_id:string,
     *     artifact_kind:string,
     *     schema_version:string,
     *     created_at:string,
     *     content_json: array<string,mixed>
     *   }>,
     *   next_cursor: string|null
     * }
     */
    public function search(array $params): array;
}