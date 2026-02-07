<?php

declare(strict_types=1);

namespace App\Modules\Auth\Application\UseCase;

use App\Modules\Auth\Domain\ValueObject\AuthPrincipal;
use App\Modules\Auth\Infrastructure\External\Auth0\Auth0ManagementApiClient;
use App\Modules\Auth\Presentation\Notification\EmailVerificationTicketNotification;
use Illuminate\Support\Facades\Notification;
use Illuminate\Support\Facades\URL;

final class SendEmailVerificationTicketUseCase
{
    public function handle(AuthPrincipal $principal): void
    {
        // Auth0 ユーザーのみ（事故防止）
        if ($principal->provider() !== AuthPrincipal::PROVIDER_AUTH0) {
            throw new \DomainException('Email verification ticket is available only for Auth0 users.');
        }

        // 宛先メールはこのユースケースでは必須
        $toEmail = $principal->requireEmail();

        // ✅ Auth0 sub は providerUid()
        $auth0UserId = $principal->providerUid();

        $resultUrl = URL::temporarySignedRoute(
    name: 'auth.verify_second.auth0',
    expiration: now()->addSeconds($ttl),
    parameters: ['sub' => $auth0UserId],
);
        $ttl = (int) config('auth0_management.verify_ttl_sec', 900);

        $client = Auth0ManagementApiClient::fromConfig();
        $ticketUrl = $client->createEmailVerificationTicket(
            auth0UserId: $auth0UserId,
            resultUrl: $resultUrl,
            ttlSec: $ttl
        );

        // Userモデルに依存せず送る（OCC流：AuthとUser分離）
        Notification::route('mail', $toEmail)
            ->notify(new EmailVerificationTicketNotification($ticketUrl));
    }
}