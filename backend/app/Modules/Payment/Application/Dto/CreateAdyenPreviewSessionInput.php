<?php

namespace App\Modules\Payment\Application\Dto;

final class CreateAdyenPreviewSessionInput
{
    public function __construct(
        public int $userId,
        public int $shopId,
        public int $amount,
        public string $currency = 'JPY',
    ) {}
}

final class CreateAdyenPreviewSessionOutput
{
    public function __construct(
        public string $previewKey,
        public string $sessionId,
        public string $sessionData,
        public string $environment,
    ) {}

    public function toArray(): array
    {
        return [
            'preview_key' => $this->previewKey,
            'session_id' => $this->sessionId,
            'session_data' => $this->sessionData,
            'environment' => $this->environment,
        ];
    }
}