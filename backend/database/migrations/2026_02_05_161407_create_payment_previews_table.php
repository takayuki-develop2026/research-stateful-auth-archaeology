<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class () extends Migration {
    public function up(): void
    {
        Schema::create('payment_previews', function (Blueprint $table) {
            $table->id();

            $table->uuid('preview_key')->unique();          // reference に使う
            $table->unsignedBigInteger('user_id')->index();
            $table->unsignedBigInteger('shop_id')->index();

            $table->string('provider', 50)->index();        // adyen
            $table->string('method', 50)->index();          // card

            $table->unsignedInteger('amount');
            $table->string('currency', 10);

            $table->string('session_id', 191)->index();
            $table->char('session_data_hash', 64);          // sha256(sessionData)
            $table->string('environment', 16);              // test|live

            $table->unsignedBigInteger('order_id')->nullable()->index();
            $table->unsignedBigInteger('payment_id')->nullable()->index();

            $table->string('status', 32)->index();          // created|committed|expired|cancelled
            $table->timestamp('expires_at')->nullable();
            $table->timestamp('committed_at')->nullable();
            $table->timestamp('cancelled_at')->nullable();

            $table->timestamps();

            $table->unique(['provider', 'session_id'], 'uq_preview_provider_session');
            $table->unique(['provider', 'preview_key'], 'uq_preview_provider_key');
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('payment_previews');
    }
};