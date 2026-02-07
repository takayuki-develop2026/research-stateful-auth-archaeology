<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration {
    public function up(): void
    {
        Schema::create('user_identity_verifications', function (Blueprint $table) {
            $table->id();

            $table->unsignedBigInteger('user_identity_id');

            // email verification only (拡張余地：phone, mfa, etc.)
            $table->string('type', 32)->default('email'); // 'email'
            $table->timestamp('verified_at')->nullable();

            // 証跡（任意だが将来効く）
            $table->string('verified_provider', 32)->nullable(); // 'auth0' | 'firebase' etc
            $table->string('verified_subject', 255)->nullable(); // sub/uid
            $table->longText('evidence_json')->nullable();

            $table->timestamps();

            $table->foreign('user_identity_id')
                ->references('id')
                ->on('user_identities')
                ->onDelete('cascade');

            // 1 identity に email verification は1行（最新版のみ）
            $table->unique(['user_identity_id', 'type']);
            $table->index(['verified_at']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('user_identity_verifications');
    }
};