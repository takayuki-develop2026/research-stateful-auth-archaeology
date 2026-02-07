<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration {
    public function up(): void
    {
        Schema::create('user_identities', function (Blueprint $table) {
            $table->id();
            $table->unsignedBigInteger('user_id');

            $table->string('provider', 32);      // firebase/auth0/cognito/custom/token
            $table->string('provider_uid', 255); // sub / uid

            $table->string('email')->nullable();
            // $table->boolean('email_verified')->nullable();
            $table->string('display_name')->nullable();

            $table->longText('claims_json')->nullable();

            $table->timestamps();

            $table->unique(['provider', 'provider_uid']);
            $table->index(['user_id']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('user_identities');
    }
};