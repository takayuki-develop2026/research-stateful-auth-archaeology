<?php

declare(strict_types=1);

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

final class UserIdentityVerification extends Model
{
    protected $table = 'user_identity_verifications';

    protected $fillable = [
        'user_identity_id',
        'type',
        'verified_at',
        'verified_provider',
        'verified_subject',
        'evidence_json',
    ];

    protected $casts = [
        'user_identity_id' => 'integer',
        'verified_at' => 'datetime',
        // longText でも JSON文字列を入れれば array cast は動く
        'evidence_json' => 'array',
        'created_at' => 'datetime',
        'updated_at' => 'datetime',
    ];

    protected $dates = [
    'verified_at',
    ];

    public function identity(): BelongsTo
    {
        return $this->belongsTo(UserIdentity::class, 'user_identity_id');
    }

    public function scopeEmail($q)
    {
        return $q->where('type', 'email');
    }

    public function getIsVerifiedAttribute(): bool
    {
        return $this->verified_at !== null;
    }
}