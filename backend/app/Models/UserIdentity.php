<?php

declare(strict_types=1);

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasMany;

final class UserIdentity extends Model
{
    protected $table = 'user_identities';

    protected $fillable = [
        'user_id',
        'provider',
        'provider_uid',
        'email',
        'display_name',
        'claims_json',
    ];

    protected $casts = [
        'user_id' => 'integer',
        // longText でも JSON文字列を入れれば array cast は普通に動く
        'claims_json' => 'array',
        'created_at' => 'datetime',
        'updated_at' => 'datetime',
    ];

    /* =========================
       Relations
    ========================= */

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class);
    }

    public function verifications(): HasMany
    {
        return $this->hasMany(UserIdentityVerification::class, 'user_identity_id');
    }

    /* =========================
       Scopes
    ========================= */

    public function scopeProviderUid($q, string $provider, string $providerUid)
    {
        return $q->where('provider', $provider)->where('provider_uid', $providerUid);
    }
}