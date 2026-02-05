<?php

namespace App\Modules\Payment\Application\Dto\Admin\TrustLedger;

final class AdminKpiDto
{
    /**
     * @param array<string, array{
     *   sales_total:int,
     *   refund_total:int,
     *   fee_total:int,
     *   net_total:int,
     *   postings_count:int
     * }> $by_provider
     */
    public function __construct(
        public readonly string $from,
        public readonly string $to,
        public readonly string $currency,
        public readonly int $sales_total,
        public readonly int $refund_total,
        public readonly int $fee_total,
        public readonly int $net_total,
        public readonly int $postings_count,
        public readonly array $by_provider = [],
    ) {
    }

    public function toArray(): array
    {
        return [
            'from' => $this->from,
            'to' => $this->to,
            'currency' => $this->currency,
            'sales_total' => $this->sales_total,
            'refund_total' => $this->refund_total,
            'fee_total' => $this->fee_total,
            'net_total' => $this->net_total,
            'postings_count' => $this->postings_count,
            'by_provider' => $this->by_provider,
        ];
    }
}