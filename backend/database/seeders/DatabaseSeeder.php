<?php

namespace Database\Seeders;

use Illuminate\Database\Seeder;

class DatabaseSeeder extends Seeder
{
    public function run()
    {
        $this->call([
            RoleSeeder::class,
        ]);

        if (app()->environment('local')) {
            $this->call(LocalUsersSeeder::class);
        } else {
            $this->call(FirebaseUsersSeeder::class); // Firebase前提
        }

        $this->call([
            UserAddressesTableSeeder::class,
            ProfilesTableSeeder::class,
            ShopsTableSeeder::class,
            ShopAddressesTableSeeder::class,
            RoleUserSeeder::class,
            ItemsTableSeeder::class,
            BrandEntitySeeder::class,
            ConditionEntitySeeder::class,
            ColorEntitySeeder::class,
            ItemDraftSeeder::class,
            AnalysisRequestSeeder::class,
            ProviderSeeder::class,
            BackfillLegacyIdentitySeeder::class,
        ]);
    }
}