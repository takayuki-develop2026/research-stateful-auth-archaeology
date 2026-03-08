<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\Hash;
use App\Models\User;

class LocalUsersSeeder extends Seeder
{
    public function run(): void
    {
        $users = [
            [
                'name' => 'テスト用のユーザ1',
                'email' => 'valid.email@example.com',
                'password' => 'Testtest1',
                'post_number' => '111-1111',
                'address' => '東京都港区芝公園4-2-8',
                'building' => 'コーポA',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
            [
                'name' => 'テスト用のユーザ2',
                'email' => 'taro.y@coachtech.com',
                'password' => 'Testtest2',
                'post_number' => '222-2222',
                'address' => '大阪府大阪市阿倍野区阿倍野筋1-1-43',
                'building' => 'ハイツB',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
            [
                'name' => 'テスト用のユーザ3',
                'email' => 'reina.n@coachtech.com',
                'password' => 'Testtest3',
                'post_number' => '333-3333',
                'address' => '福岡県福岡市東区香椎照葉6-2-52',
                'building' => 'エトワールC',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
            [
                'name' => 'テスト用のユーザ4',
                'email' => 'tomomi.a@coachtech.com',
                'password' => 'Testtest4',
                'post_number' => '444-4444',
                'address' => '北海道札幌市中央区北五条西2丁目',
                'building' => 'エスポワールD',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
            [
                'name' => 'テスト用のユーザ5',
                'email' => 'pro.t@coachtech.com',
                'password' => 'Testtest5',
                'post_number' => '326-1234',
                'address' => '栃木県',
                'building' => '足利',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
            [
                'name' => '川田隆之',
                'email' => 't.principle.k2024@gmail.com',
                'password' => 'Takayuki1',
                'post_number' => '326-0000',
                'address' => '栃木県',
                'building' => '足利 FacilityCenter',
                'address_country' => 'JP',
                'shop_id' => null,
            ],
        ];

        // 既存ローカルユーザーを整理（任意）
        User::whereIn('email', collect($users)->pluck('email'))->delete();

        foreach ($users as $user) {
            User::create([
                'name' => $user['name'],
                'email' => $user['email'],
                'password' => Hash::make($user['password']),
                'post_number' => $user['post_number'],
                'address' => $user['address'],
                'building' => $user['building'],
                'address_country' => $user['address_country'],
                'shop_id' => $user['shop_id'],
                'profile_completed' => true,
                'firebase_uid' => null,              // ← ローカルの決定打
                'email_verified_at' => now(),        // ← 確認済み扱い
                'first_login_at' => now(),
            ]);
        }
    }
}