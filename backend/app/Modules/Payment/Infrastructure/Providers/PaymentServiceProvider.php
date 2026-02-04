<?php

namespace App\Modules\Payment\Infrastructure\Providers;

use Illuminate\Support\ServiceProvider;

use App\Modules\Payment\Domain\Repository\Wallet\WalletRepository;
use App\Modules\Payment\Domain\Repository\Wallet\StoredPaymentMethodRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Wallet\EloquentWalletRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Wallet\EloquentStoredPaymentMethodRepository;

use App\Modules\Payment\Domain\Service\PaymentMethodVault;
use App\Modules\Payment\Infrastructure\Gateway\StripePaymentMethodVault;

use App\Modules\Payment\Domain\Ledger\Repository\LedgerPostingRepository;
use App\Modules\Payment\Domain\Ledger\Repository\LedgerEntryRepository;
use App\Modules\Payment\Domain\Ledger\Service\LedgerPoster;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentLedgerPostingRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentLedgerEntryRepository;

use App\Modules\Payment\Domain\Ledger\Repository\LedgerQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentLedgerQueryRepository;

use App\Modules\Payment\Domain\Port\FeeQueryPort;
use App\Modules\Payment\Infrastructure\Gateway\StripeFeeQuery;

use App\Modules\Payment\Domain\Ledger\Repository\LedgerReconciliationQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentLedgerReconciliationQueryRepository;

use App\Modules\Payment\Domain\Ledger\Port\PostLedgerPort;
use App\Modules\Payment\Infrastructure\Ledger\LocalPostLedgerPort;
use App\Modules\Payment\Infrastructure\Ledger\HttpPostLedgerPort;

use App\Modules\Payment\Domain\Account\Repository\AccountRepository;
use App\Modules\Payment\Domain\Account\Repository\BalanceRepository;
use App\Modules\Payment\Domain\Account\Repository\LedgerBalanceQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentAccountRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentBalanceRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentLedgerBalanceQueryRepository;

use App\Modules\Payment\Domain\Account\Repository\HoldRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentHoldRepository;

use App\Modules\Payment\Domain\Account\Repository\PayoutRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentPayoutRepository;

use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerKpiQueryRepository;
use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerPostingQueryRepository;
use App\Modules\Payment\Domain\Ledger\Repository\AdminLedgerReconciliationQueryRepository;
use App\Modules\Payment\Domain\Ledger\Repository\AdminWebhookEventQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentAdminLedgerKpiQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentAdminLedgerPostingQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentAdminLedgerReconciliationQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Ledger\EloquentAdminWebhookEventQueryRepository;

use App\Modules\Payment\Domain\Account\Repository\AdminHoldQueryRepository;
use App\Modules\Payment\Domain\Account\Repository\AdminPayoutQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentAdminHoldQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Account\EloquentAdminPayoutQueryRepository;

use App\Modules\Payment\Domain\Port\PaymentGatewayPort;
use App\Modules\Payment\Infrastructure\Gateway\StripePaymentGateway;
use App\Modules\Payment\Infrastructure\Gateway\AdyenPaymentGateway;
use App\Modules\Payment\Domain\Enum\PaymentProvider;

use App\Modules\Payment\Domain\Shop\Repository\AdminShopQueryRepository;
use App\Modules\Payment\Infrastructure\Persistence\Repository\Shop\EloquentAdminShopQueryRepository;


final class PaymentServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        // ✅ v2-5.1: PostLedgerPort は 1回だけ bind（feature flag 有効化）
        $this->app->bind(PostLedgerPort::class, function ($app) {
    $driver = env('LEDGER_POST_DRIVER', config('trustledger.ledger.driver', 'local'));

    return match ($driver) {
        'http' => $app->make(HttpPostLedgerPort::class),
        default => $app->make(LocalPostLedgerPort::class),
    };
});

        $this->app->bind(PaymentMethodVault::class, StripePaymentMethodVault::class);

        $this->app->bind(WalletRepository::class, EloquentWalletRepository::class);
        $this->app->bind(StoredPaymentMethodRepository::class, EloquentStoredPaymentMethodRepository::class);

        $this->app->bind(LedgerPostingRepository::class, EloquentLedgerPostingRepository::class);
        $this->app->bind(LedgerEntryRepository::class, EloquentLedgerEntryRepository::class);
        $this->app->singleton(LedgerPoster::class, fn () => new LedgerPoster());

        $this->app->bind(LedgerQueryRepository::class, EloquentLedgerQueryRepository::class);
        $this->app->bind(FeeQueryPort::class, StripeFeeQuery::class);
        $this->app->bind(LedgerReconciliationQueryRepository::class, EloquentLedgerReconciliationQueryRepository::class);

        $this->app->bind(AccountRepository::class, EloquentAccountRepository::class);
        $this->app->bind(BalanceRepository::class, EloquentBalanceRepository::class);
        $this->app->bind(LedgerBalanceQueryRepository::class, EloquentLedgerBalanceQueryRepository::class);

        $this->app->bind(HoldRepository::class, EloquentHoldRepository::class);
        $this->app->bind(PayoutRepository::class, EloquentPayoutRepository::class);

        $this->app->bind(AdminLedgerKpiQueryRepository::class, EloquentAdminLedgerKpiQueryRepository::class);
        $this->app->bind(AdminLedgerPostingQueryRepository::class, EloquentAdminLedgerPostingQueryRepository::class);
        $this->app->bind(AdminLedgerReconciliationQueryRepository::class, EloquentAdminLedgerReconciliationQueryRepository::class);
        $this->app->bind(AdminWebhookEventQueryRepository::class, EloquentAdminWebhookEventQueryRepository::class);

        $this->app->bind(AdminHoldQueryRepository::class, EloquentAdminHoldQueryRepository::class);
        $this->app->bind(AdminPayoutQueryRepository::class, EloquentAdminPayoutQueryRepository::class);

        $this->app->bind(AdminShopQueryRepository::class, EloquentAdminShopQueryRepository::class);

        $this->app->bind(PaymentGatewayPort::class, function ($app) {
    $p = (string) env('PAYMENT_PROVIDER', PaymentProvider::STRIPE->value);

    return match ($p) {
        PaymentProvider::ADYEN->value => $app->make(AdyenPaymentGateway::class),
        default => $app->make(StripePaymentGateway::class),
    };
});
    }
}