<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;
use App\Models\Role;
use App\Models\Shop;
use App\Models\User;

class RoleUserSeeder extends Seeder
{
    public function run()
    {
        DB::table('role_user')->truncate();

        $ownerRole = Role::where('slug', 'owner')->first();
        $adminRole = Role::where('slug', 'domain_lead_admin')->first();

        if (! $ownerRole) {
            return;
        }

        // owner: ユーザーとショップを「存在する分だけ」対応付け
        $users = User::orderBy('id')->take(4)->get();
        $shops = Shop::orderBy('id')->take(4)->get();

        foreach ($users as $index => $user) {
            $shop = $shops[$index] ?? null;

            if (! $shop) {
                continue;
            }

            DB::table('role_user')->insert([
                'user_id' => $user->id,
                'role_id' => $ownerRole->id,
                'shop_id' => $shop->id,
                'created_at' => now(),
                'updated_at' => now(),
            ]);
        }

        // domain_lead_admin（shop 非依存）を複数ユーザーに付与
        if ($adminRole) {
            $adminUsers = User::whereIn('email', [
                't.principle.k2024@gmail.com',
                'pro.t@coachtech.com',
            ])->get();

            foreach ($adminUsers as $adminUser) {
                DB::table('role_user')->insert([
                    'user_id' => $adminUser->id,
                    'role_id' => $adminRole->id,
                    'shop_id' => null,
                    'created_at' => now(),
                    'updated_at' => now(),
                ]);
            }
        }
    }
}