<?php

namespace App\Modules\Payment\Application\UseCase\Wallet;

use App\Modules\Payment\Application\Dto\Wallet\CompleteSetupIntentOutput;
use App\Modules\Payment\Domain\Repository\Wallet\WalletRepository;
use App\Modules\Payment\Domain\Repository\Wallet\StoredPaymentMethodRepository;
use App\Modules\Payment\Domain\Service\PaymentMethodVault;
use App\Modules\Payment\Domain\Entity\Wallet\CustomerWallet;
use Illuminate\Support\Facades\DB;

final class CompleteSetupIntentUseCase
{
    public function __construct(
        private WalletRepository $wallets,
        private StoredPaymentMethodRepository $methods,
        private PaymentMethodVault $vault,
    ) {
    }

    /**
     * SetupIntent を確定し、payment_method を stored_payment_methods に反映する
     */
    public function handle(
        int $userId,
        string $setupIntentId,
        string $provider = 'stripe',
        bool $makeDefault = true, // v1固定：保存したカードを default に寄せる
    ): CompleteSetupIntentOutput {
        return DB::transaction(function () use ($userId, $setupIntentId, $provider, $makeDefault) {

            // 1) wallet
            $wallet = $this->wallets->findByUserId($userId, $provider);
            if (! $wallet || $wallet->id() === null) {
                $wallet = $this->wallets->create(
                    CustomerWallet::createForUser($userId, provider: $provider)
                );
            }

            // 2) Stripe側 SetupIntent を取得
            $snap = $this->vault->retrieveSetupIntent($setupIntentId);

            if ($snap->status !== 'succeeded') {
                throw new \RuntimeException("SetupIntent not succeeded: {$snap->status}");
            }

            $pmId = $snap->providerPaymentMethodId;
            if (!is_string($pmId) || $pmId === '') {
                throw new \RuntimeException('SetupIntent has no payment_method');
            }

            // 3) カードスナップショット取得（表示・監査用）
            $card = $this->vault->retrievePaymentMethodCard($pmId);

            // 4) upsert
            $this->methods->upsertActiveCard(
                walletId: (int)$wallet->id(),
                provider: $provider,
                providerPaymentMethodId: $pmId,
                brand: $card->brand,
                last4: $card->last4,
                expMonth: $card->expMonth,
                expYear: $card->expYear,
            );

            // 5) default 方針
            // - 初回は repo が default=true にする
            // - 2枚目以降も v1 は「今回保存したカードを default」に寄せる（makeDefault=true）
            if ($makeDefault) {
                // upsert後の row id を引ける手段がないので、
                // 最小で追加：findDefaultActiveRow を使って "今回pmがdefaultになっているか" を見つつ、
                // ならない場合は "今回pmの row id" を探して default にする
                $rowId = $this->findActiveRowIdByProviderPm((int)$wallet->id(), $provider, $pmId);
                if ($rowId !== null) {
                    $this->methods->setDefault((int)$wallet->id(), $rowId);
                }
            }

            $isDefault = false;
            $def = $this->methods->findDefaultActiveRow((int)$wallet->id());
            if ($def && (string)$def['provider_payment_method_id'] === $pmId) {
                $isDefault = true;
            }

            return new CompleteSetupIntentOutput(
                ok: true,
                walletId: (int)$wallet->id(),
                provider: $provider,
                providerPaymentMethodId: $pmId,
                isDefault: $isDefault,
            );
        });
    }

    /**
     * Repositoryに "findIdByProviderPaymentMethodId" を増やさず最小で済ませたいので、
     * ここではDB直叩きで id を引く（v1段階の妥協点）
     *
     * ※ 純粋DDDに寄せるなら StoredPaymentMethodRepository に findId... を追加してください。
     */
    private function findActiveRowIdByProviderPm(int $walletId, string $provider, string $providerPaymentMethodId): ?int
    {
        $row = DB::table('stored_payment_methods')
            ->where('wallet_id', $walletId)
            ->where('provider', $provider)
            ->where('provider_payment_method_id', $providerPaymentMethodId)
            ->where('status', 'active')
            ->first();

        return $row ? (int)$row->id : null;
    }
}