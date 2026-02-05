<?php

namespace App\Modules\Payment\Application\UseCase\Admin\TrustLedger;

use App\Modules\Payment\Application\Dto\Admin\TrustLedger\CursorPageDto;
use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerPostingQueryRepository;

final class SearchPostingsUseCase
{
    public function __construct(
        private AdminLedgerPostingQueryRepository $postings,
    ) {
    }

    public function handle(
        ?array $shopIds,
        string $from,
        string $to,
        string $currency,
        string $postingType,
        string $sourceProvider, // ✅ 追加
        ?string $q,
        ?int $paymentId,
        ?int $orderId,
        ?string $sourceEventId,
        int $limit,
        ?string $cursor
    ): CursorPageDto {
        $r = $this->postings->searchPostings(
            $shopIds,
            $from,
            $to,
            $currency,
            $postingType,
            $sourceProvider,
            $q,
            $paymentId,
            $orderId,
            $sourceEventId,
            $limit,
            $cursor
        );

        return new CursorPageDto($r['items'], $r['next_cursor']);
    }
}