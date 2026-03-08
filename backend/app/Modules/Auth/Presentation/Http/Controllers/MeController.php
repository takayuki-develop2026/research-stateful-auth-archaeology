<?php

namespace App\Modules\Auth\Presentation\Http\Controllers;

use App\Http\Controllers\Controller;
use App\Modules\Auth\Application\Context\AuthContext;
use App\Modules\Auth\Infrastructure\External\Auth0\Auth0ManagementApiClient;
use App\Models\Shop;
use App\Models\UserIdentity;
use App\Models\UserIdentityVerification;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;

final class MeController extends Controller
{
    public function __construct(
        private AuthContext $authContext,
    ) {
    }

    public function __invoke(Request $request): JsonResponse
    {
        $user = $request->user();
        $principal = $this->authContext->principal();

        $ctx = [
            'path' => $request->path(),
            'method' => $request->method(),
            'ip' => $request->ip(),
        ];

        if (! $user || ! $principal) {
            Log::warning('[Me] unauthenticated', $ctx + [
                'has_user' => (bool) $user,
                'has_principal' => (bool) $principal,
            ]);

            return response()->json(['message' => 'Unauthenticated'], 401);
        }

        $ctx += [
            'user_id' => (int) $user->id,
            'provider' => (string) $principal->provider(),
            'provider_uid' => (string) $principal->providerUid(),
            'principal_email' => (string) ($principal->email() ?? ''),
        ];

        Log::info('[Me] start', $ctx);

        $secondVerifiedAt = $this->findSecondVerifiedAt((int) $user->id);

        Log::info('[Me] db lookup email_second', $ctx + [
            'second_verified_at' => $secondVerifiedAt ? (string) $secondVerifiedAt : null,
        ]);

        if (! $secondVerifiedAt && $principal->provider() === 'auth0') {
            Log::info('[Me] auth0 sync attempt', $ctx);

            try {
                $auth0UserId = (string) $principal->providerUid();
                $mgmt = Auth0ManagementApiClient::fromConfig();

                Log::info('[Me] auth0 mgmt getUser start', $ctx + [
                    'auth0_user_id' => $auth0UserId,
                ]);

                $auth0User = $mgmt->getUser($auth0UserId);
                $emailVerified = (bool) ($auth0User['email_verified'] ?? false);

                Log::info('[Me] auth0 mgmt getUser ok', $ctx + [
                    'auth0_email' => $auth0User['email'] ?? null,
                    'auth0_email_verified' => $emailVerified,
                ]);

                if ($emailVerified) {
                    $identity = UserIdentity::query()
                        ->where('provider', 'auth0')
                        ->where('provider_uid', $auth0UserId)
                        ->first();

                    Log::info('[Me] identity lookup', $ctx + [
                        'identity_found' => (bool) $identity,
                        'identity_id' => $identity?->id,
                    ]);

                    if (! $identity) {
                        $identity = UserIdentity::query()->create([
                            'user_id' => (int) $user->id,
                            'provider' => 'auth0',
                            'provider_uid' => $auth0UserId,
                            'email' => $auth0User['email'] ?? $principal->email(),
                            'display_name' => $auth0User['name'] ?? $user->name,
                            'claims_json' => $auth0User,
                        ]);

                        Log::info('[Me] identity created', $ctx + [
                            'identity_id' => (int) $identity->id,
                        ]);
                    }

                    $row = UserIdentityVerification::query()->updateOrCreate(
                        [
                            'user_identity_id' => (int) $identity->id,
                            'type' => 'email_second',
                        ],
                        [
                            'verified_at' => now(),
                            'verified_provider' => 'auth0',
                            'verified_subject' => $auth0UserId,
                            'evidence_json' => [
                                'source' => 'auth0_mgmt_users',
                                'email_verified' => true,
                                'email' => $auth0User['email'] ?? null,
                            ],
                        ]
                    );

                    Log::info('[Me] email_second upserted', $ctx + [
                        'verification_id' => (int) $row->id,
                        'identity_id' => (int) $identity->id,
                        'type' => (string) $row->type,
                        'verified_at' => $row->verified_at ? (string) $row->verified_at : null,
                    ]);

                    $secondVerifiedAt = $this->findSecondVerifiedAt((int) $user->id);

                    Log::info('[Me] db re-check email_second', $ctx + [
                        'second_verified_at' => $secondVerifiedAt ? (string) $secondVerifiedAt : null,
                    ]);
                } else {
                    Log::info('[Me] auth0 email not verified yet', $ctx);
                }
            } catch (\Throwable $e) {
                Log::warning('[Me] auth0 sync failed', $ctx + [
                    'error' => $e->getMessage(),
                ]);
            }
        }

        if (! $secondVerifiedAt) {
            Log::info('[Me] second verification required (403)', $ctx);

            return response()->json([
                'message' => 'Second email verification required',
                'code' => 'email_second_required',
            ], 403);
        }

        $emailVerifiedAt = Carbon::parse($secondVerifiedAt)->toIso8601String();

        Log::info('[Me] ok (200)', $ctx + [
            'email_verified_at' => $emailVerifiedAt,
        ]);

        $allRoles = $user->roles()->get(['roles.id', 'roles.slug', 'roles.name']);

        $shopScopedRoles = $allRoles->filter(
            fn ($role) => ! is_null($role->pivot->shop_id)
        );

        $globalRoles = $allRoles
            ->filter(fn ($role) => is_null($role->pivot->shop_id))
            ->map(fn ($role) => [
                'role_id' => (int) $role->id,
                'role' => $role->slug,
                'name' => $role->name,
            ])
            ->values();

        $shopIds = $shopScopedRoles->pluck('pivot.shop_id')->unique()->values();

        $shopCodeById = Shop::query()
            ->whereIn('id', $shopIds)
            ->pluck('shop_code', 'id');

        $shopRoles = $shopScopedRoles
            ->map(fn ($role) => [
                'role_id'   => (int) $role->id,
                'shop_id'   => (int) $role->pivot->shop_id,
                'shop_code' => $shopCodeById[$role->pivot->shop_id] ?? null,
                'role'      => $role->slug,
                'name'      => $role->name,
            ])
            ->values();

        return response()->json([
            'id' => (int) $user->id,
            'email' => $principal->email(),
            'display_name' => $user->name,
            'email_verified_at' => $emailVerifiedAt,
            'profile_completed' => (bool) $user->profile_completed,
            'shop_roles' => $shopRoles,
            'global_roles' => $globalRoles,
        ]);
    }

    private function findSecondVerifiedAt(int $userId): mixed
    {
        return DB::table('user_identity_verifications as v')
            ->join('user_identities as i', 'i.id', '=', 'v.user_identity_id')
            ->where('i.user_id', $userId)
            ->where('v.type', 'email_second')
            ->whereNotNull('v.verified_at')
            ->orderByDesc('v.verified_at')
            ->orderByDesc('v.id')
            ->value('v.verified_at');
    }
}