<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class PaymentPreview extends Model
{
    use HasFactory;

    /**
     * 一括割り当て（Mass Assignment）を許可する属性
     */
    protected $fillable = [
        'preview_key',
        'user_id',
        'shop_id',
        'provider',
        'method',
        'amount',
        'currency',
        'session_id',
        'session_data_hash',
        'environment',
        'order_id',
        'payment_id',
        'status',
        'expires_at',
    ];

    /**
     * 属性の型変換（キャスト）
     * 日付や数値、UUIDなどを扱いやすくします
     */
    protected $casts = [
        'amount' => 'integer',
        'expires_at' => 'datetime',
        'user_id' => 'integer',
        'shop_id' => 'integer',
        'order_id' => 'integer',
        'payment_id' => 'integer',
    ];

    /**
     * 有効期限が切れているか判定するスコープやメソッド（便利機能）
     */
    public function isExpired(): bool
    {
        return $this->expires_at && $this->expires_at->isPast();
    }
}