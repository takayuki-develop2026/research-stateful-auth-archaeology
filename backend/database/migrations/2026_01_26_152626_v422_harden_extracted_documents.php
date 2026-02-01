<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class () extends Migration {
    public function up(): void
    {
        Schema::table('extracted_documents', function (Blueprint $table) {
            // ① 取得バイナリ（or HTML body）のsha256
            $table->char('raw_hash', 64)->nullable()->after('source_url_hash')->index();

            // ② 実行条件（将来 engine router で増える）
            $table->string('engine', 32)->default('')->after('raw_hash')->index();
            $table->string('mode', 16)->default('')->after('engine')->index();
            $table->string('lang', 32)->default('')->after('mode')->index();

            // ③ アルゴリズム世代（将来の後戻り回避の要）
            $table->string('pipeline_version', 16)->default('v4.2')->after('lang')->index();

            // ④ 旧ユニークは強すぎるので落とす（※必須）
            $table->dropUnique('uk_domain_content_hash');

            // ⑤ 新ユニーク：raw基準 + 実行条件 + 世代
            $table->unique(
                ['domain', 'source_url_hash', 'raw_hash', 'engine', 'mode', 'lang', 'pipeline_version'],
                'uk_domain_source_raw_pipeline'
            );
        });
    }

    public function down(): void
    {
        Schema::table('extracted_documents', function (Blueprint $table) {
            $table->dropUnique('uk_domain_source_raw_pipeline');

            // 旧ユニークを戻す（ロールバック用）
            $table->unique(['domain', 'content_hash'], 'uk_domain_content_hash');

            $table->dropColumn(['raw_hash', 'engine', 'mode', 'lang', 'pipeline_version']);
        });
    }
};