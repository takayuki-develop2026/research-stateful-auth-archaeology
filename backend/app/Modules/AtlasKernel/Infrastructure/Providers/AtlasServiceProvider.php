<?php

declare(strict_types=1);

namespace App\Modules\AtlasKernel\Infrastructure\Providers;

use Illuminate\Support\ServiceProvider;

use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactQueryRepository;
use App\Modules\AtlasKernel\Infrastructure\Persistence\Repository\PgsqlRunArtifactQueryRepository;

use App\Modules\AtlasKernel\Domain\Contracts\RunArtifacts\RunArtifactAdminQueryRepository;
use App\Modules\AtlasKernel\Infrastructure\Persistence\Repository\PgsqlRunArtifactAdminQueryRepository;

use App\Modules\AtlasKernel\Domain\Repository\DecisionLedgerRepository;
use App\Modules\AtlasKernel\Infrastructure\Persistence\Repository\EloquentDecisionLedgerRepository;

final class AtlasServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        // RunArtifacts (read-only)
        $this->app->bind(
            RunArtifactQueryRepository::class,
            fn () => new PgsqlRunArtifactQueryRepository(connectionName: 'ak_pgsql')
        );

        // Existing DecisionLedger (keep as-is)
        $this->app->bind(
            DecisionLedgerRepository::class,
            EloquentDecisionLedgerRepository::class
        );

        $this->app->bind(
    RunArtifactAdminQueryRepository::class,
    fn () => new PgsqlRunArtifactAdminQueryRepository(connectionName: 'ak_pgsql')
);
    }
}