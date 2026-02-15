<?php
declare(strict_types=1);

namespace App\Modules\AtlasKernel\Application\UseCase\RunArtifacts\Admin;

use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactAdminQueryRepository;
use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\ArtifactKind;

final class ListAdminRunArtifactsUseCase
{
    public function __construct(
        private readonly RunArtifactAdminQueryRepository $repo,
    ) {}

    /** @return array{items:array, known_kinds:array<int,string>, next_cursor:?string} */
    public function handle(?string $kind, ?string $q, int $limit, ?string $cursor): array
    {
        // kind があるならフォーマットだけは先に弾く（artifact_kind のみ）
        if ($kind !== null && trim($kind) !== '') {
            ArtifactKind::assertValid($kind);
        }

        $res = $this->repo->search([
            'kind' => $kind,
            'q' => $q,
            'limit' => $limit,
            'cursor' => $cursor,
        ]);

        return [
            'items' => $res['items'],
            'known_kinds' => ArtifactKind::knownKinds(),
            'next_cursor' => $res['next_cursor'],
        ];
    }
}