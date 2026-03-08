<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;
use App\Models\User;

class ProfilesTableSeeder extends Seeder
{
    public function run(): void
    {
        /**
         * UsersTableSeeder と完全に揃えたテストプロフィール
         */
        $profiles = [
            [
                'email'        => 'valid.email@example.com',
                'display_name' => 'テスト用のユーザ１',
                'post_number'  => '111-1111',
                'address'      => '東京都港区芝公園4-2-8',
                'building'     => 'コーポA',
                'user_image'   => null,
            ],
            [
                'email'        => 'taro.y@coachtech.com',
                'display_name' => 'テスト用のユーザ2',
                'post_number'  => '222-2222',
                'address'      => '大阪府大阪市阿倍野区阿倍野筋1-1-43',
                'building'     => 'ハイツB',
                'user_image'   => null,
            ],
            [
                'email'        => 'reina.n@coachtech.com',
                'display_name' => 'テスト用のユーザ3',
                'post_number'  => '333-3333',
                'address'      => '福岡県福岡市東区香椎照葉6-2-52',
                'building'     => 'エトワールC',
                'user_image'   => null,
            ],
            [
                'email'        => 'tomomi.a@coachtech.com',
                'display_name' => 'テスト用のユーザ4',
                'post_number'  => '444-4444',
                'address'      => '北海道札幌市中央区北五条西2丁目',
                'building'     => 'エスポワールD',
                'user_image'   => null,
            ],
            [
                'email'        => 'pro.t@coachtech.com',
                'display_name' => 'テスト用のユーザ5',
                'post_number'  => '326-1234',
                'address'      => '栃木県',
                'building'     => '足利',
                'user_image'   => null,
            ],
            [
                'email'        => 't.principle.k2024@gmail.com',
                'display_name' => '川田隆之',
                'post_number'  => '326-0000',
                'address'      => '栃木県',
                'building'     => '足利 FacilityCenter',
                'user_image'   => null,
            ],
        ];

        foreach ($profiles as $data) {
            $user = User::where('email', $data['email'])->first();

            if (! $user) {
                Log::warning("Profile skipped (user not found): {$data['email']}");
                continue;
            }

            DB::table('profiles')->updateOrInsert(
                ['user_id' => $user->id],
                [
                    'display_name' => $data['display_name'],
                    'post_number'  => $data['post_number'],
                    'address'      => $data['address'],
                    'building'     => $data['building'],
                    'user_image'   => $data['user_image'],
                    'created_at'   => now(),
                    'updated_at'   => now(),
                ]
            );

            Log::info("Profile seeded for user_id={$user->id} ({$data['email']})");
        }
    }
}
