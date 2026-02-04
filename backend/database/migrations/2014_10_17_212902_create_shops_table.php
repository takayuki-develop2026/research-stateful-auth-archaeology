<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class () extends Migration {
    public function up(): void
    {
        Schema::create('shops', function (Blueprint $table) {
            $table->id();

            // 店舗名
            $table->string('name', 50);

            // 店舗コード（短い識別子で URL や API に使う）

            $table->string('shop_code', 64)->unique();


            // 店舗のオーナーとなるユーザー
            // ★ 修正: foreignId を使用しているため、constrained('users') を有効化し、外部キー制約を設定
            $table->foreignId('owner_user_id')
                ->nullable()
                  ->constrained('users') // 外部キー制約を有効化
                  ->nullOnDelete();   // 関連ユーザー削除時にショップも削除されるように設定


            $table->enum('type', ['personal', 'business'])
                ->default('personal');


            // 店舗ステータス
            $table->enum('status', ['active', 'inactive'])->default('active');

            $table->string('banner_url')->nullable();

            // 任意の説明文
            $table->string('description')->nullable();

            // 店舗ロゴ
            $table->string('logo')->nullable();

            // 決済プロバイダー情報
            $table->string('payment_provider', 32)
                ->default('stripe');
                // 検索性が欲しければ index
            $table->index('payment_provider');

            $table->json('payment_provider_meta')->nullable();
            $table->timestamp('payment_provider_updated_at')->nullable();

            $table->timestamps();
        });
    }

    public function down(): void
    {
        // 外部キー制約を削除してからテーブルをドロップするのが安全
        Schema::table('shops', function (Blueprint $table) {
            $table->dropConstrainedForeignId('owner_user_id');
        });

        Schema::dropIfExists('shops');

        Schema::table('shops', function (Blueprint $table) {
            $table->dropColumn([
                'payment_provider',
                'payment_provider_meta',
                'payment_provider_updated_at',
            ]);
        });
    }
};
