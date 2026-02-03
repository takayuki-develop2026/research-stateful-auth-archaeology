<?php

declare(strict_types=1);

namespace App\Modules\Order\Application\UseCase;

use App\Modules\Order\Domain\Event\OrderPaid;
use App\Modules\Order\Domain\Repository\OrderRepository;

final class MarkOrderPaidUseCase
{
    public function __construct(
        private OrderRepository $orders,
    ) {
    }

    /**
     * @return OrderPaid|null すでにpaidならnull
     */
    public function handle(int $orderId, \DateTimeImmutable $paidAt): ?OrderPaid
    {
        $order = $this->orders->findById($orderId);
        if (! $order) {
            throw new \RuntimeException('Order not found');
        }

        // 冪等：すでに paid なら何もしない
        if ($order->isPaid()) {
            return null;
        }

        // Domainルール（Order::markPaid 内で status チェック等が走る）
        $paidOrder = $order->markPaid($paidAt);
        $this->orders->save($paidOrder);

        return new OrderPaid(
            orderId: $paidOrder->id(),
            shopId: $paidOrder->shopId(),
        );
    }
}