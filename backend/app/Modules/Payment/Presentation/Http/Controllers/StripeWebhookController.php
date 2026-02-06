<?php

namespace App\Modules\Payment\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\UseCase\HandlePaymentWebhookUseCase;
use App\Modules\Payment\Application\UseCase\Wallet\HandleWalletWebhookUseCase;
use App\Modules\Payment\Application\Dto\HandlePaymentWebhookInput;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;
use Illuminate\Support\Facades\DB;
use Stripe\Webhook;

final class StripeWebhookController extends Controller
{
    public function __construct(
        private HandlePaymentWebhookUseCase $paymentUseCase,
        private HandleWalletWebhookUseCase $walletUseCase,
    ) {
    }

    public function __invoke(Request $request): Response
    {
        $payload = $request->getContent();
$sig     = $request->header('Stripe-Signature');
$secret  = config('services.stripe.webhook_secret');

try {
    $event = Webhook::constructEvent($payload, $sig, $secret);
} catch (\Throwable $e) {
    \Log::warning('[Stripe Webhook Invalid Signature]', [
        'message' => $e->getMessage(),
    ]);
    return response('Invalid signature', 400);
}

$occurredAt = (new \DateTimeImmutable('@' . $event->created))
    ->setTimezone(new \DateTimeZone(config('app.timezone')));

// ✅ provider event id（Stripeの一意ID）
$providerEventId = (string) $event->id; // evt_...

// ✅ internal stable event id（hex64）
$eventId = hash('sha256', 'stripe|' . $providerEventId . '|' . (string) $event->type);

$input = new HandlePaymentWebhookInput(
    provider: 'stripe',
    eventId: $eventId,                     // ✅ sha256 hex64
    providerEventId: $providerEventId,      // ✅ evt_...
    eventType: (string) $event->type,
    payload: $event->toArray(),
    payloadHash: hash('sha256', $payload),
    occurredAt: $occurredAt,
);
\Log::info('[StripeWebhook] payload saved', [
    'event_id' => $input->eventId,
    'event_type' => $input->eventType,
    'payload_len' => strlen($payload),
]);
        // ✅ 受信イベントをDB保存（Replay用）
// 既存行がある場合は payload を埋める（今回のような payload=null を救済）
$raw = $payload; // すでに $payload = $request->getContent();

$hash = hash('sha256', $raw);

$ids = $this->extractIdsFromStripePayload($input->payload);

$existing = DB::table('payment_webhook_events')
    ->where('provider', 'stripe')
    ->where('event_id', $input->eventId)
    ->first();

if ($existing) {
    DB::table('payment_webhook_events')
        ->where('id', $existing->id)
        ->update([
            'event_type'   => $input->eventType,
            'payload_hash' => $hash,
            'payload'      => $raw,
            'signature'    => $sig,

            // ✅ 既存がNULLなら埋める（上書きしない）
            'payment_id'   => $existing->payment_id ?? $ids['payment_id'],
            'order_id'     => $existing->order_id ?? $ids['order_id'],
            'provider_event_id' => $existing->provider_event_id ?? $input->providerEventId,

            // statusは受信時点ではreceivedでOK（reserve/completeで更新される想定）
            'status'       => $existing->status ?? 'received',

            'updated_at'   => now(),
        ]);
} else {
    DB::table('payment_webhook_events')->insert([
        'provider'      => 'stripe',
        'event_id'      => $input->eventId,
        'provider_event_id' => $input->providerEventId,   // evt_
        'event_type'    => $input->eventType,
        'payload_hash'  => $hash,
        'payload'       => $raw,
        'signature'     => $sig,
        'status'        => 'received',

        // ✅ ここで埋める
        'payment_id'    => $ids['payment_id'],
        'order_id'      => $ids['order_id'],

        'created_at'    => now(),
        'updated_at'    => now(),
    ]);
}

        try {
            // Wallet系
            if (str_starts_with($event->type, 'setup_intent.') || str_starts_with($event->type, 'payment_method.')) {
                $this->walletUseCase->handle($input);
            } else {
                $this->paymentUseCase->handle($input);
            }
        } catch (\Throwable $e) {
            \Log::error('[Stripe Webhook UseCase Throwable Swallowed]', [
                'event_id'   => $input->eventId,
                'event_type' => $input->eventType,
                'message'    => $e->getMessage(),
            ]);
        }

        return response()->json(['ok' => true], 200);
    }

    private function extractIdsFromStripePayload(array $payload): array
    {
        $object = $payload['data']['object'] ?? null;
        if (!is_array($object)) {
            return ['payment_id' => null, 'order_id' => null, 'shop_id' => null];
        }

        $meta = $object['metadata'] ?? null;
        if (!is_array($meta)) {
            return ['payment_id' => null, 'order_id' => null, 'shop_id' => null];
        }

        $paymentId = isset($meta['payment_id']) && is_numeric($meta['payment_id'])
            ? (int)$meta['payment_id']
            : null;

        $orderId = isset($meta['order_id']) && is_numeric($meta['order_id'])
            ? (int)$meta['order_id']
            : null;

        $shopId = isset($meta['shop_id']) && is_numeric($meta['shop_id'])
            ? (int)$meta['shop_id']
            : null;

        return ['payment_id' => $paymentId, 'order_id' => $orderId, 'shop_id' => $shopId];
    }
}