<?php

declare(strict_types=1);

namespace App\Modules\Auth\Presentation\Notification;

use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Notifications\Messages\MailMessage;
use Illuminate\Notifications\Notification;

final class EmailVerificationTicketNotification extends Notification implements ShouldQueue
{
    use Queueable;

    public function __construct(
        private readonly string $ticketUrl
    ) {
    }

    public function via(mixed $notifiable): array
    {
        return ['mail'];
    }

    public function toMail(mixed $notifiable): MailMessage
    {
        return (new MailMessage)
            ->subject('メールアドレス確認のお願い')
            ->line('以下のボタンからメールアドレス確認を完了してください。')
            ->action('メールアドレスを確認する', $this->ticketUrl)
            ->line('このリンクは一定時間で無効になります。');
    }
}