<?php

namespace App\Modules\Payment\Application\UseCase\Admin\TrustLedger;

use App\Modules\Payment\Application\Dto\Admin\TrustLedger\AdminShopKpiRowDto;
use App\Modules\Payment\Application\Dto\Admin\TrustLedger\CursorPageDto;
use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerKpiQueryRepository;

final class GetShopKpisUseCase
{
    public function __construct(
        private AdminLedgerKpiQueryRepository $kpis,
    ) {
    }

    /**
     * shop_ids を指定しない場合は「返せるだけ返す」（最小）
     * ※ shop一覧ページングは後で ShopQuery と統合して強化
     */
    public function handle(?array $shopIds, string $from, string $to, string $currency): CursorPageDto
    {
        $map = $this->kpis->getShopKpis($shopIds, $from, $to, $currency);

        $items = [];
        foreach ($map as $row) {
            $sales = (int)($row['sales'] ?? 0);
            $refund = (int)($row['refund'] ?? 0);
            $fee = (int)($row['fee'] ?? 0);
            $count = (int)($row['postings_count'] ?? 0);

            $byProvider = [];
            $byProviderRaw = $row['by_provider'] ?? [];

            if (is_array($byProviderRaw)) {
                foreach ($byProviderRaw as $provider => $pRow) {
                    if (!is_array($pRow)) continue;

                    $ps = (int)($pRow['sales'] ?? 0);
                    $pr = (int)($pRow['refund'] ?? 0);
                    $pf = (int)($pRow['fee'] ?? 0);
                    $pc = (int)($pRow['postings_count'] ?? 0);

                    $byProvider[(string)$provider] = [
                        'sales_total' => $ps,
                        'refund_total' => $pr,
                        'fee_total' => $pf,
                        'net_total' => $ps - $pr - $pf,
                        'postings_count' => $pc,
                    ];
                }
            }

            $dto = new AdminShopKpiRowDto(
                shop_id: (int)$row['shop_id'],
                from: $from,
                to: $to,
                currency: $currency,
                sales_total: $sales,
                refund_total: $refund,
                fee_total: $fee,
                net_total: $sales - $refund - $fee,
                postings_count: $count,
                by_provider: $byProvider,
            );

            $items[] = $dto->toArray();
        }

        // 最小：ページングは shopIds 指定時のみ想定（next_cursor=null）
        return new CursorPageDto($items, null);
    }
}