<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

class CreateItemsTable extends Migration
{
    public function up()
    {
        Schema::create('items', function (Blueprint $table) {
            $table->id();

            /**
             * 出品起源（★最重要）
             */

            $table->enum('item_origin', [
                'user_personal',
                'shop_managed',
            ]);


            /**
             * ショップに属する場合のみ入る
             */
            $table->foreignId('shop_id')
    ->nullable()
    ->constrained('shops')
    ->nullOnDelete();

            /**
             * 個人出品者
             */
            $table->foreignId('created_by_user_id')
                ->nullable()
                ->constrained('users')
                ->nullOnDelete();

            /* =========================
             * 商品情報（Fact）
             * ========================= */
            $table->string('name', 30);

            /**
             * 価格（SoT）
             * publish 時に必ず確定
             */
            $table->integer('price')
                ->comment('price amount (JPY), NOT NULL on publish');

            /**
             * 将来の多通貨対応余地
             */
            $table->char('price_currency', 3)
                ->default('JPY');

            $table->string('brand', 20)->nullable();
            $table->string('explain', 255);
            $table->string('condition', 20);
            $table->json('category');

            /**
             * 表示用画像（draft は入れない）
             */
            $table->string('item_image')->nullable();

            /**
             * 在庫数
             */
            $table->unsignedInteger('remain')->default(1);

            $table->index(['shop_id', 'published_at', 'remain']);

            /* =========================
             * 公開制御（★今回の核心）
             * ========================= */

            /**
             * 公開日時
             * - null = 下書き / 非公開
             * - not null = 公開済み
             */
            $table->timestamp('published_at')
                ->nullable()
                ->index();

            $table->timestamps();

            /* =========================
             * インデックス
             * ========================= */
            $table->index(['item_origin', 'shop_id']);
            $table->index(['created_by_user_id']);
        });
    }

    public function down()
    {
        Schema::dropIfExists('items');
    }
}
