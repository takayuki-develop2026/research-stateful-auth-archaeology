<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Modules\Payment\Domain\Repository\PaymentQueryRepository;
use Illuminate\Http\JsonResponse;

final class GetWebhookEventDetailController
{
    public function __construct(
        private PaymentQueryRepository $webhookEvents,
    ) {
    }

    public function __invoke(string $id): JsonResponse
{
    $isHex64 = (bool) preg_match('/\A[a-f0-9]{64}\z/i', $id);

    $row = $isHex64
        ? $this->webhookEvents->findWebhookEventByIdempotencyKey($id)  // sha256
        : $this->webhookEvents->findWebhookEventByEventId($id);        // evt_...

    if (!$row) {
        return response()->json([
            'error_type' => 'NotFound',
            'message' => 'Webhook event not found.',
            'id' => $id,
        ], 404);
    }

    return response()->json($row);
}
}