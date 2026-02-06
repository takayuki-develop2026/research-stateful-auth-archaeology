<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class () extends Migration {
    public function up(): void
    {
        Schema::create('processed_webhook_events', function (Blueprint $table) {
    $table->id();

    $table->string('provider', 50);                 // stripe|adyen
    $table->char('event_id', 64);                   // sha256 hex (fixed)
    $table->string('event_type', 191)->index();     // payment_intent.succeeded / AUTHORISATION ...
    $table->char('payload_hash', 64)->index();      // sha256(payload)
    $table->string('status', 32)->index();          // reserved|ok|ignored|error など
    $table->unsignedBigInteger('payment_id')->nullable()->index();
    $table->unsignedBigInteger('order_id')->nullable()->index();
    $table->string('provider_event_id', 191)->nullable()->index();
    $table->index(['provider', 'provider_event_id'], 'ix_processed_provider_provider_event');
    $table->string('error_code', 64)->nullable();
    $table->string('error_message', 255)->nullable();

    $table->timestamp('processed_at')->nullable();
    $table->timestamps();

    $table->unique(['provider', 'event_id'], 'uq_processed_webhook_provider_event');
});
    }

    public function down(): void
    {
        Schema::dropIfExists('processed_webhook_events');
    }
};
