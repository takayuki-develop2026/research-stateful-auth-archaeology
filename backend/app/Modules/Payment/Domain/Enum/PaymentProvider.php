<?php

namespace App\Modules\Payment\Domain\Enum;

enum PaymentProvider: string
{
    case STRIPE = 'stripe';
    case ADYEN  = 'adyen';
}
