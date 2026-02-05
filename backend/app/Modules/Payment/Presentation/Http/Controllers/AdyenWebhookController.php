<?php

namespace App\Modules\Payment\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Application\UseCase\HandlePaymentWebhookUseCase;
use App\Modules\Payment\Infrastructure\Gateway\AdyenHmacVerifier;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

final class AdyenWebhookController extends Controller
{
    public function __construct(
        private HandlePaymentWebhookUseCase $paymentUseCase,
    ) {}

    public function __invoke(Request $request): Response
    {
        $payload = $request->json()->all();

        $items = $payload['notificationItems'] ?? [];
        if (!is_array($items)) {
            return response('[accepted]', 200);
        }

        $verifier = new AdyenHmacVerifier((string) config('services.adyen.hmac_key', ''));

        foreach ($items as $wrapper) {
            $nri = $wrapper['NotificationRequestItem'] ?? null;
            if (!is_array($nri)) continue;

            // HMAC verify (fail => ignore, but return [accepted])
            try {
                if (!$verifier->verify($nri)) {
                    \Log::warning('[AdyenWebhook] HMAC verification failed', [
                        'eventCode' => $nri['eventCode'] ?? null,
                        'pspReference' => $nri['pspReference'] ?? null,
                    ]);
                    continue;
                }
            } catch (\Throwable $e) {
                \Log::warning('[AdyenWebhook] HMAC verifier error', [
                    'message' => $e->getMessage(),
                ]);
                continue;
            }

            $eventCode = (string)($nri['eventCode'] ?? '');
            $success   = (string)($nri['success'] ?? '');
            $pspRef    = (string)($nri['pspReference'] ?? '');
            $eventDate = (string)($nri['eventDate'] ?? '');
            $merchantRef = (string)($nri['merchantReference'] ?? '');

            // ✅ eventId: 安定キー（pspRef + eventCode + success + merchantReference）
            $eventId = hash('sha256', 'adyen|' . $pspRef . '|' . $eventCode . '|' . $success . '|' . $merchantRef);

            // ✅ payloadHash: NRI単位（items複数に対応）
            $payloadHash = hash('sha256', json_encode($nri, JSON_UNESCAPED_UNICODE));

            $occurredAt = new \DateTimeImmutable(
                $eventDate ?: 'now',
                new \DateTimeZone((string) config('app.timezone'))
            );

            $input = new HandlePaymentWebhookInput(
                provider: 'adyen',
                eventId: $eventId,
                eventType: $eventCode,
                payload: $nri,
                payloadHash: $payloadHash,
                occurredAt: $occurredAt,
            );

            try {
                $this->paymentUseCase->handle($input);
            } catch (\Throwable $e) {
                \Log::error('[AdyenWebhook] UseCase error swallowed', [
                    'event_id' => $eventId,
                    'event_code' => $eventCode,
                    'message' => $e->getMessage(),
                ]);
                \Log::info('[AdyenWebhook] nri', [
                    'eventCode' => $nri['eventCode'] ?? null,
                    'success' => $nri['success'] ?? null,
                    'pspReference' => $nri['pspReference'] ?? null,
                    'merchantReference' => $nri['merchantReference'] ?? null,
                ]);
            }
        }

        return response('[accepted]', 200);
    }
}