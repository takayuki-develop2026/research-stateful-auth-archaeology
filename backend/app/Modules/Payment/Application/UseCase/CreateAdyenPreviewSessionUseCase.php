<?php

namespace App\Modules\Payment\Application\UseCase;

use App\Modules\Payment\Application\Dto\CreateAdyenPreviewSessionInput;
use App\Modules\Payment\Application\Dto\CreateAdyenPreviewSessionOutput;
use App\Modules\Payment\Domain\Enum\PaymentMethod;
use App\Modules\Payment\Domain\Enum\PaymentProvider;
use App\Modules\Payment\Domain\Port\PaymentGatewayPort;
use App\Modules\Payment\Domain\Repository\PaymentPreviewRepository;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Str;

final class CreateAdyenPreviewSessionUseCase
{
    public function __construct(
        private PaymentPreviewRepository $previews,
        private PaymentGatewayPort $adyenGateway, // contextual binding で Adyen 固定
    ) {}

    public function handle(CreateAdyenPreviewSessionInput $in): CreateAdyenPreviewSessionOutput
    {
        return DB::transaction(function () use ($in) {
            $previewKey = (string) Str::uuid();

            // ✅ Adyen の merchantReference = reference = previewKey
            $ctx = [
                'reference' => $previewKey,
                'preview_key' => $previewKey,
                'user_id' => $in->userId,
                'shop_id' => $in->shopId,
            ];

            $res = $this->adyenGateway->createSession(
                method: PaymentMethod::CARD,
                amount: $in->amount,
                currency: $in->currency,
                context: $ctx,
            );

            $sessionId = (string)($res['session_id'] ?? '');
            $sessionData = (string)($res['session_data'] ?? '');
            $env = (string)($res['environment'] ?? 'test');

            if ($sessionId === '' || $sessionData === '') {
                throw new \RuntimeException('Adyen session response invalid');
            }

            $this->previews->create([
                'preview_key' => $previewKey,
                'user_id' => $in->userId,
                'shop_id' => $in->shopId,
                'provider' => PaymentProvider::ADYEN->value,
                'method' => PaymentMethod::CARD->value,
                'amount' => $in->amount,
                'currency' => strtoupper($in->currency),
                'session_id' => $sessionId,
                'session_data_hash' => hash('sha256', $sessionData),
                'environment' => $env === 'live' ? 'live' : 'test',
                'status' => 'created',
                'expires_at' => now()->addMinutes(15),
            ]);

            return new CreateAdyenPreviewSessionOutput(
                previewKey: $previewKey,
                sessionId: $sessionId,
                sessionData: $sessionData,
                environment: $env === 'live' ? 'live' : 'test',
            );
        });
    }
}