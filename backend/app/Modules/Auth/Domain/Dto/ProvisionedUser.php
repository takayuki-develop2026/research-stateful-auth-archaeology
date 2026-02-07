<?php

namespace App\Modules\Auth\Domain\Dto;

final class ProvisionedUser
{
    public function __construct(
        public readonly int $userId,
        public readonly ?string $email,
        public readonly bool $emailVerified,
        public readonly ?string $emailVerifiedAt, // ✅ 追加（ISO8601 or null）
        public readonly bool $isFirstLogin,
        public readonly array $shopIds,
        public readonly array $roles = [],        // ✅ slug 配列
        public readonly ?int $tenantId = null,
    ) {}
}