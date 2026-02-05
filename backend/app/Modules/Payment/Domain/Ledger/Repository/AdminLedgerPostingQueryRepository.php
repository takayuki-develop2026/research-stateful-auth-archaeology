<?php

namespace App\Modules\Payment\Domain\Ledger\Repository;

interface AdminLedgerPostingQueryRepository
{
    public function searchPostings(
    ?array $shopIds,
    string $from,
    string $to,
    string $currency,
    string $postingType,
    string $sourceProvider,
    ?string $q,
    ?int $paymentId,
    ?int $orderId,
    ?string $sourceEventId,
    int $limit,
    ?string $cursor
): array;

    public function getPostingDetail(int $postingId): array;
}