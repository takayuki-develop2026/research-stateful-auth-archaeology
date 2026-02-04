<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Shop extends Model
{
    use HasFactory;

    protected $table = 'shops';

    protected $fillable = [
  'shop_code','owner_user_id','name','type','status','description','logo','banner_url',
  'payment_provider','payment_provider_meta','payment_provider_updated_at',
];

protected $casts = [
  'payment_provider_meta' => 'array',
  'payment_provider_updated_at' => 'datetime',
];

    public function shippingAddress()
    {
        return $this->hasOne(\App\Models\ShopAddress::class, 'shop_id');
    }
}
