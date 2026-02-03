<?php

namespace App\Modules\Payment\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use App\Modules\Payment\Application\UseCase\HandlePaymentWebhookUseCase;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

final class AdyenWebhookController extends Controller
{
    public function __construct(
        private HandlePaymentWebhookUseCase $paymentUseCase,
    ) {}

    public function __invoke(Request $request): Response
    {
        $payloadRaw = $request->getContent();
        $payload = $request->json()->all();

        // ✅ Adyenは通知に対して "[accepted]" を返す必要がある
        $items = $payload['notificationItems'] ?? [];
        if (!is_array($items)) {
            return response('[accepted]', 200);
        }

        foreach ($items as $wrapper) {
            $nri = $wrapper['NotificationRequestItem'] ?? null;
            if (!is_array($nri)) continue;

            $eventCode = (string)($nri['eventCode'] ?? '');
            $success   = (string)($nri['success'] ?? '');
            $pspRef    = (string)($nri['pspReference'] ?? '');
            $eventDate = (string)($nri['eventDate'] ?? '');

            // 冪等キー（provider+pspReference+eventCode+success+eventDate）
            $eventId = hash('sha256', 'adyen|' . $pspRef . '|' . $eventCode . '|' . $success . '|' . $eventDate);

            $occurredAt = new \DateTimeImmutable($eventDate ?: 'now', new \DateTimeZone(config('app.timezone')));

            $input = new HandlePaymentWebhookInput(
                provider: 'adyen',
                eventId: $eventId,
                eventType: $eventCode,           // 例: AUTHORISATION
                payload: $payload,               // 全体も渡す（監査用）
                payloadHash: hash('sha256', $payloadRaw),
                occurredAt: $occurredAt,
            );

            try {
                $this->paymentUseCase->handle($input);
            } catch (\Throwable $e) {
                \Log::error('[Adyen Webhook UseCase Throwable Swallowed]', [
                    'event_id' => $eventId,
                    'event_code' => $eventCode,
                    'message' => $e->getMessage(),
                ]);
            }
        }

        return response('[accepted]', 200);
    }
}