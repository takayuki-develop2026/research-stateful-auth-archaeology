<?php

declare(strict_types=1);

namespace App\Modules\Auth\Domain\ValueObject;

use App\Modules\Auth\Domain\Dto\ProvisionedUser;
use App\Models\User;

final class AuthPrincipal
{
    public const PROVIDER_SANCTUM  = 'sanctum';
    public const PROVIDER_FIREBASE = 'firebase';
    public const PROVIDER_AUTH0    = 'auth0';
    public const PROVIDER_INTERNAL = 'internal';
    public const PROVIDER_JWT      = 'jwt'; // composite verifier で “provider不明” を使うなら

    private function __construct(
        private int $userId,
        private ?string $email,
        private string $provider,
        private string $providerUid,
        private bool $emailVerified,
        private ?string $emailVerifiedAt,
        private array $roles = [],
        private array $shopRoles = [],
    ) {
        $this->provider  = self::normalizeProvider($this->provider);
        $this->providerUid = self::normalizeNonEmptyString($this->providerUid, 'providerUid');

        $this->email = self::normalizeNullableEmail($this->email);

        $this->roles     = self::normalizeRoleSlugs($this->roles);
        $this->shopRoles = self::normalizeShopRoles($this->shopRoles);

        $this->emailVerifiedAt = self::normalizeNullableIso8601($this->emailVerifiedAt);

        $this->assertInvariant();
    }

    /* =====================================================
     * Factory methods
     * ===================================================== */

    public static function fromSanctumUser(User $user): self
    {
        $verifiedAt = $user->email_verified_at
            ? $user->email_verified_at->toISOString()
            : null;

        return new self(
            userId: (int) $user->id,
            email: is_string($user->email) ? $user->email : null,
            provider: self::PROVIDER_SANCTUM,
            providerUid: (string) $user->id,
            emailVerified: (bool) $user->email_verified_at,
            emailVerifiedAt: $verifiedAt,
            roles: [],
            shopRoles: [],
        );
    }

    public static function fromProvisionedUser(
        ProvisionedUser $user,
        string $provider,
        string $providerUid,
        array $shopRoles = [],
    ): self {
        return new self(
            userId: (int) $user->userId,
            email: is_string($user->email) ? $user->email : null,
            provider: $provider,
            providerUid: $providerUid,
            emailVerified: (bool) $user->emailVerified,
            emailVerifiedAt: $user->emailVerifiedAt, // ✅ 分離SoTの結果をそのまま
            roles: $user->roles ?? [],
            shopRoles: $shopRoles,
        );
    }

    public static function fromJwtPayload(
        int $userId,
        ?string $email,
        string $provider,
        string $providerUid,
        bool $emailVerified,
        ?string $emailVerifiedAt = null,
        array $roles = [],
        array $shopRoles = [],
    ): self {
        return new self(
            userId: $userId,
            email: $email,
            provider: $provider,
            providerUid: $providerUid,
            emailVerified: $emailVerified,
            emailVerifiedAt: $emailVerifiedAt,
            roles: $roles,
            shopRoles: $shopRoles,
        );
    }

    /* =====================================================
     * Accessors
     * ===================================================== */

    public function userId(): int { return $this->userId; }
    public function email(): ?string { return $this->email; }
    public function provider(): string { return $this->provider; }
    public function providerUid(): string { return $this->providerUid; }
    public function isEmailVerified(): bool { return $this->emailVerified; }
    public function emailVerifiedAt(): ?string { return $this->emailVerifiedAt; }
    public function roles(): array { return $this->roles; }
    public function shopRoles(): array { return $this->shopRoles; }

    /* =====================================================
     * Guards
     * ===================================================== */

    public function requireEmailVerified(): void
    {
        if (! $this->emailVerified) {
            throw new \DomainException('Email not verified');
        }
    }

    /**
     * ✅ email を必須にしたいユースケースだけで使う
     * （Auth0/Firebaseで email を取れない構成や、将来の電話番号主語などにも耐える）
     */
    public function requireEmail(): string
    {
        if (!is_string($this->email) || $this->email === '') {
            throw new \DomainException('Email is required');
        }
        return $this->email;
    }

    public function requireRole(string $role): void
    {
        if (! in_array($role, $this->roles, true)) {
            throw new \DomainException("Role '{$role}' is required");
        }
    }

    public function requireShopRole(int $shopId, string $role): void
    {
        $roles = $this->shopRoles[$shopId] ?? [];
        if (! in_array($role, $roles, true)) {
            throw new \DomainException("Shop role '{$role}' is required");
        }
    }

    /* =====================================================
     * Debug
     * ===================================================== */

    public function toArray(): array
    {
        return [
            'user_id'           => $this->userId,
            'email'             => $this->email,
            'provider'          => $this->provider,
            'provider_uid'      => $this->providerUid,
            'email_verified'    => $this->emailVerified,
            'email_verified_at' => $this->emailVerifiedAt,
            'roles'             => $this->roles,
            'shop_roles'        => $this->shopRoles,
        ];
    }

    /* =====================================================
     * Internal validation / normalization
     * ===================================================== */

    private static function normalizeProvider(string $provider): string
    {
        $p = strtolower(trim($provider));
        if ($p === '') {
            throw new \DomainException('AuthPrincipal: provider is required');
        }

        $allowed = [
            self::PROVIDER_SANCTUM,
            self::PROVIDER_FIREBASE,
            self::PROVIDER_AUTH0,
            self::PROVIDER_INTERNAL,
            self::PROVIDER_JWT,
        ];

        if (!in_array($p, $allowed, true)) {
            throw new \DomainException("AuthPrincipal: unknown provider '{$p}'");
        }

        return $p;
    }

    private static function normalizeNonEmptyString(string $v, string $field): string
    {
        $s = trim($v);
        if ($s === '') {
            throw new \DomainException("AuthPrincipal: {$field} is required");
        }
        return $s;
    }

    private static function normalizeNullableEmail(?string $email): ?string
    {
        if (!is_string($email)) return null;
        $e = trim($email);
        if ($e === '') return null;

        // 厳密バリデーションは不要だが、明らかに壊れた値は落とす
        if (strpos($e, '@') === false) return null;

        return $e;
    }

    private static function normalizeNullableIso8601(?string $v): ?string
    {
        if (!is_string($v)) return null;
        $s = trim($v);
        if ($s === '') return null;

        // 形式の厳密性はここでは保証しない（SoT側がtimestamp）
        // ただし明らかに壊れてるのは弾く（監査を汚さない）
        if (strlen($s) < 10) return null;

        return $s;
    }

    /**
     * @param array<mixed> $roles
     * @return array<int,string>
     */
    private static function normalizeRoleSlugs(array $roles): array
    {
        $out = [];
        foreach ($roles as $r) {
            if (!is_string($r)) continue;
            $slug = trim($r);
            if ($slug === '') continue;
            $out[] = $slug;
        }
        return array_values(array_unique($out));
    }

    /**
     * @param array<mixed> $shopRoles
     * @return array<int,array<int,string>>
     */
    private static function normalizeShopRoles(array $shopRoles): array
    {
        $out = [];
        foreach ($shopRoles as $shopId => $roles) {
            if (!is_int($shopId) && !ctype_digit((string)$shopId)) continue;
            $sid = (int) $shopId;
            if (!is_array($roles)) continue;
            $out[$sid] = self::normalizeRoleSlugs($roles);
        }
        return $out;
    }

    private function assertInvariant(): void
    {
        if ($this->userId <= 0) {
            throw new \DomainException('AuthPrincipal: userId must be positive');
        }

        // provider/providerUid は constructor で non-empty 化済み

        // 整合性：verified=false なのに verifiedAt があるのは矛盾
        if ($this->emailVerified === false && $this->emailVerifiedAt !== null) {
            throw new \DomainException('AuthPrincipal: verifiedAt must be null when emailVerified=false');
        }

        // 逆方向（verified=true なら verifiedAt 必須）にしないのがポイント：
        // - 移行期
        // - IdPが “trueだけ返すが時刻を返さない” ケース
        // - legacy token
        // などが現実にあるため
    }
}