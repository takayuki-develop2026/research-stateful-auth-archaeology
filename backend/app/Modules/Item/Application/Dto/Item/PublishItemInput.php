<?php

namespace App\Modules\Item\Application\Dto\Item;

final class PublishItemInput
{
    public function __construct(
        public readonly string $draftId,
        public readonly ?int $shopId,
    ) {
    }
}
