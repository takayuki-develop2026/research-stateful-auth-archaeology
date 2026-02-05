<?php

namespace App\Modules\Payment\Application\Dto;

final class AdyenCommitInput
{
    public function __construct(
        public int $userId,
        public string $previewKey,
        public int $shopId,
        public array $items,
        public int $addressId,
        public ?array $meta = null,
    ) {}
}