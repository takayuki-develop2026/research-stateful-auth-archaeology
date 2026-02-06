<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class () extends Migration {
    public function up(): void
    {
        Schema::create('payment_webhook_events', function (Blueprint $table) {
            $table->id();

            $table->string('provider', 50);
            $table->string('event_id', 255);
            $table->string('event_type', 100);
            $table->string('payload_hash', 64);

            $table->string('status', 50);
            $table->unsignedBigInteger('payment_id')->nullable();
            $table->unsignedBigInteger('order_id')->nullable();
            $table->string('provider_event_id', 191)->nullable()->index();
            $table->index(['provider', 'provider_event_id'], 'ix_processed_provider_provider_event');
            $table->text('error_message')->nullable();

            $table->timestamps();

            // ★ 冪等性の要
            $table->unique(['provider', 'event_id']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('payment_webhook_events');
    }
};
