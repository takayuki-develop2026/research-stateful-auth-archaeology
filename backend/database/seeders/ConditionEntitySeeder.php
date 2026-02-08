<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

final class ConditionEntitySeeder extends Seeder
{
    public function run(): void
    {
        $conditions = [
            // ===== 新品系 =====
            '新品' => [
                '新品',
                'しんぴん',
                'シンピン',
                'new',
                'brand new',
                '未開封新品',
            ],

            '未使用' => [
                '未使用',
                'みしよう',
                'ミシヨウ',
                'unused',
                '未着用',
            ],

            'アウトレット' => [
                'アウトレット',
                'outlet',
                '型落ち',
                '展示品',
                '展示品アウトレット',
            ],

            // ===== 美品系 =====
            '新品同様' => [
                '新品同様',
                'ほぼ新品',
                '新品に近い',
                'like new',
            ],

            '極上美品' => [
                '極上美品',
                '非常に綺麗',
                '極美品',
                'mint',
            ],

            '美品' => [
                '美品',
                'びひん',
                'ビヒン',
                'きれい',
                '綺麗',
                'clean',
                'good condition',
            ],

            'やや美品' => [
                'やや美品',
                '比較的綺麗',
                '多少使用感あり（美品）',
            ],

            // =====高級系 =====
            'プレミアム' => [
                'プレミアム',
                '高級',
                '限定版',
                'premium',
                'limited edition',
            ],

            'プレオウンド' => [
                'プレオウンド',
                'Pre-owned',
                '認定中古',
                'オーナー品',
            ],

            'ヴィンテージ' => [
                'ヴィンテージ',
                'vintage',
                '希少中古',
                '年代物',
            ],

            // ===== まあまあ系 =====
            'リユース' => [
                'リユース',
                'reuse',
                '再循環品',
                'セカンドハンド',
                'second hand',
            ],

            '中古' => [
                '中古',
                'ちゅうこ',
                'チュウコ',
                'used',
            ],

            '普通' => [
                '普通',
                'ふつう',
                '可',
                '使用感あり（普通）',
                'used - good',
            ],

            // ===== 使用感あり =====
            '使用感あり' => [
                '使用感あり',
                '使用感有',
                '使われています',
                'used',
            ],

            '傷あり' => [
                '傷あり',
                'キズあり',
                'きずあり',
                '擦り傷あり',
                '小傷あり',
                'scratched',
            ],

            '汚れあり' => [
                '汚れあり',
                'よごれあり',
                'シミあり',
                '汚れ有',
                'stained',
            ],

            // ===== 劣化系 =====
            '古い' => [
                '古い',
                'ふるい',
                '経年劣化',
                'old',
                'aged',
            ],

            '劣化あり' => [
                '劣化あり',
                '劣化',
                '色褪せ',
                '変色あり',
                'damaged',
            ],

            // ===== ジャンク・部品 =====
            '動作未確認' => [
                '動作未確認',
                '未確認',
                'operation unchecked',
            ],

            '不良品' => [
                '不良品',
                '故障',
                '壊れています',
                'broken',
                'defective',
            ],

            '部品取り' => [
                '部品取り',
                'パーツ用',
                'for parts',
            ],
        ];

        foreach ($conditions as $canonical => $synonyms) {
            $this->upsertCondition($canonical, $synonyms);
        }
    }

    private function upsertCondition(string $canonical, array $synonyms): void
    {
        $normalized = $this->normalize($canonical);

        DB::table('condition_entities')->updateOrInsert(
            ['normalized_key' => $normalized],
            [
                'canonical_name' => $canonical,
                'display_name'   => $canonical,
                'normalized_key' => $normalized,
                'synonyms_json'  => json_encode(
                    array_values(array_unique(array_map([$this, 'normalize'], $synonyms))),
                    JSON_UNESCAPED_UNICODE
                ),
                'is_primary'   => true,
                'created_from' => 'seed',
                'created_at'   => now(),
                'updated_at'   => now(),
            ]
        );
    }

    private function normalize(string $s): string
    {
        $s = trim($s);
        $s = preg_replace('/\s+/u', ' ', $s) ?? $s;
        return mb_strtolower($s, 'UTF-8');
    }
}