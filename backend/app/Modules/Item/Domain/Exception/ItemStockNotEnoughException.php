<?php

declare(strict_types=1);

namespace App\Modules\Item\Domain\Exception;

use RuntimeException;

final class ItemStockNotEnoughException extends RuntimeException
{
    /**
     * @param array<int, array{item_id:int, requested:int, remain:int}> $details
     */
    public function __construct(
        public readonly array $details,
        string $message = 'Item stock not enough',
    ) {
        parent::__construct($message);
    }
}