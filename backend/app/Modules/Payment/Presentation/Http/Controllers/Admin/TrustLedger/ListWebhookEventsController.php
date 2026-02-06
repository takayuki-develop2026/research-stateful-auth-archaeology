<?php

namespace App\Modules\Payment\Presentation\Http\Controllers\Admin\TrustLedger;

use App\Http\Controllers\Controller;
use App\Modules\Payment\Application\UseCase\Admin\TrustLedger\ListWebhookEventsUseCase;
use Illuminate\Http\Request;

final class ListWebhookEventsController extends Controller
{
    public function __construct(
        private ListWebhookEventsUseCase $useCase,
    ) {
    }

    public function __invoke(Request $request)
    {
        $data = $request->validate([
            'from' => 'required|date_format:Y-m-d',
            'to' => 'required|date_format:Y-m-d',
            'status' => 'sometimes|string|in:ok,ignored,error,reserved',
            'event_type' => 'sometimes|string',
            'q' => 'sometimes|string',
            'limit' => 'sometimes|integer|min:1|max:200',
            'cursor' => 'sometimes|string',
        ]);

        $page = $this->useCase->handle(
            from: $data['from'],
            to: $data['to'],
            status: $data['status'] ?? null,
            eventType: $data['event_type'] ?? null,
            q: $data['q'] ?? null,
            limit: (int)($data['limit'] ?? 50),
            cursor: $data['cursor'] ?? null,
        );


        return response()->json($page->toArray(), 200);
    }
}