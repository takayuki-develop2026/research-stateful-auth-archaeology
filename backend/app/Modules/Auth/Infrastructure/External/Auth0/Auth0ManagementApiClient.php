<?php

declare(strict_types=1);

namespace App\Modules\Auth\Infrastructure\External\Auth0;

use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Http;
use RuntimeException;

final class Auth0ManagementApiClient
{
    public function __construct(
        private readonly string $domain,
        private readonly string $clientId,
        private readonly string $clientSecret,
        private readonly string $audience,
    ) {
    }

    public static function fromConfig(): self
    {
        $domain = (string) config('auth0_management.domain');
        $clientId = (string) config('auth0_management.client_id');
        $clientSecret = (string) config('auth0_management.client_secret');
        $audience = (string) config('auth0_management.audience');

        if ($domain === '' || $clientId === '' || $clientSecret === '' || $audience === '') {
            throw new RuntimeException('Auth0 management config missing (domain/client_id/client_secret/audience).');
        }

        return new self($domain, $clientId, $clientSecret, $audience);
    }

    /** client_credentials で Management API token を取得（Cacheで再利用） */
    public function getAccessToken(): string
    {
        $cacheKey = 'auth0.mgmt.token.' . sha1($this->domain . '|' . $this->audience);

        $cached = Cache::get($cacheKey);
        if (is_string($cached) && $cached !== '') return $cached;

        $url = "https://{$this->domain}/oauth/token";

        $res = Http::asJson()
            ->timeout(10)
            ->post($url, [
                'grant_type' => 'client_credentials',
                'client_id' => $this->clientId,
                'client_secret' => $this->clientSecret,
                'audience' => $this->audience,
            ]);

        if (!$res->successful()) {
            throw new RuntimeException('Auth0 oauth/token failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();
        $token = $json['access_token'] ?? null;
        $expiresIn = (int) ($json['expires_in'] ?? 0);

        if (!is_string($token) || $token === '') {
            throw new RuntimeException('Auth0 oauth/token missing access_token.');
        }

        $ttl = max(60, $expiresIn - 60);
        Cache::put($cacheKey, $token, $ttl);

        return $token;
    }

    /**
     * Email Verification Ticket を生成（result_url で戻り先固定）
     * scope: create:user_tickets
     */
    public function createEmailVerificationTicket(
        string $auth0UserId,
        string $resultUrl,
        int $ttlSec = 900
    ): string {
        $token = $this->getAccessToken();
        $url = "https://{$this->domain}/api/v2/tickets/email-verification";

        $res = Http::asJson()
            ->withToken($token)
            ->timeout(10)
            ->post($url, [
                'user_id' => $auth0UserId,
                'result_url' => $resultUrl,
                'ttl_sec' => $ttlSec,
            ]);

        if (!$res->successful()) {
            throw new RuntimeException('Auth0 tickets/email-verification failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();
        $ticketUrl = $json['ticket'] ?? null;

        if (!is_string($ticketUrl) || $ticketUrl === '') {
            throw new RuntimeException('Auth0 ticket response missing ticket url.');
        }

        return $ticketUrl;
    }

    /**
     * Auth0 User を取得（email_verified を確認するため）
     * scope: read:users
     */
    public function getUser(string $auth0UserId): array
    {
        $token = $this->getAccessToken();
        $encoded = rawurlencode($auth0UserId);
        $url = "https://{$this->domain}/api/v2/users/{$encoded}";

        $res = Http::asJson()
            ->withToken($token)
            ->timeout(10)
            ->get($url);

        if (!$res->successful()) {
            throw new RuntimeException('Auth0 get user failed: ' . $res->status() . ' ' . $res->body());
        }

        $json = $res->json();
        if (!is_array($json)) {
            throw new RuntimeException('Auth0 get user response invalid json.');
        }

        return $json;
    }
}