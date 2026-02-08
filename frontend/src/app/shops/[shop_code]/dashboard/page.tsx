"use client";

import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { useParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";
import { useAuthGuard } from "@/ui/auth/useAuthGuard";

function subscribeLastLoginAt(onStoreChange: () => void) {
  const onChanged = () => onStoreChange();
  const onStorage = (e: StorageEvent) => {
    if (e.key === "last_login_at") onStoreChange();
  };
  const onFocus = () => onStoreChange();
  const onVisibility = () => onStoreChange();

  window.addEventListener("occ:last_login_at_changed", onChanged);
  window.addEventListener("storage", onStorage);
  window.addEventListener("focus", onFocus);
  document.addEventListener("visibilitychange", onVisibility);

  return () => {
    window.removeEventListener("occ:last_login_at_changed", onChanged);
    window.removeEventListener("storage", onStorage);
    window.removeEventListener("focus", onFocus);
    document.removeEventListener("visibilitychange", onVisibility);
  };
}

function getLastLoginAtSnapshot(): string | null {
  try {
    return localStorage.getItem("last_login_at");
  } catch {
    return null;
  }
}

// SSR用（Client Componentでも要求されることがあるので同じでOK）
function getLastLoginAtServerSnapshot(): string | null {
  return null;
}

export default function ShopDashboardPage() {
  // useAuthGuard({
  //   requireAuth: true,
  //   requireVerified: true,
  //   requireProfile: true,
  //   allowJustLoggedInBypass: true, // owner dashboard の瞬間だけバイパス
  // });

  const params = useParams();
  const shop_code = (params as any)?.shop_code as string;

  const router = useRouter();
  const {
    user,
    isAuthenticated,
    isLoading: isAuthLoading,
    authReady,
    apiClient,
  } = useAuth() as any;

  const [shopName, setShopName] = useState<string | null>(null);

  // ✅ これが「リアルタイム表示の本体」
  const loginAt = useSyncExternalStore(
    subscribeLastLoginAt,
    getLastLoginAtSnapshot,
    getLastLoginAtServerSnapshot,
  );

  // shop 名取得（ここはそのまま）
  useEffect(() => {
    if (!authReady || isAuthLoading) return;
    if (!isAuthenticated) return;
    if (!shop_code) return;

    (apiClient.get("/shops/me") as Promise<any>)
      .then((res) => {
        const shops = Array.isArray(res)
          ? res
          : Array.isArray(res?.shops)
            ? res.shops
            : res?.shop
              ? [res.shop]
              : res?.shop_code
                ? [res]
                : [];
        const hit = shops.find((s: any) => s.shop_code === shop_code);
        setShopName(hit?.name ?? null);
      })
      .catch(() => setShopName(null));
  }, [authReady, isAuthLoading, isAuthenticated, shop_code, apiClient]);

  if (!authReady || isAuthLoading)
    return <div className="p-6">読み込み中...</div>;
  if (!isAuthenticated) {
    router.replace("/login");
    return null;
  }

  const roleInShop =
    user?.shop_roles?.find((r: any) => r.shop_code === shop_code)?.role ?? null;

  const isShopStaff =
    user?.shop_roles?.some(
      (r: any) =>
        r.shop_code === shop_code &&
        ["owner", "manager", "staff"].includes(r.role),
    ) ?? false;

  if (!isShopStaff)
    return <div className="p-6">アクセス権限がありません。</div>;

  return (
    <div className="p-6 space-y-4">
      <Link href={`/shops/${shop_code}`} className="text-blue-600 underline">
        ← 店舗トップへ戻る
      </Link>

      <div className="space-y-1">
        <h1 className="text-3xl font-bold">店舗ダッシュボード</h1>

        <p className="text-sm text-gray-600">
          <b>
            店舗名: {shopName ?? shop_code} / ユーザー名:{" "}
            {user?.display_name ?? user?.email ?? "-"}（{roleInShop ?? "-"}）
          </b>
        </p>

        <p className="text-xs text-gray-500">
          ログイン時刻: {loginAt ? new Date(loginAt).toLocaleString() : "-"}
        </p>
      </div>

      {/* 2列×3行（=合計6枠） */}
      <div className="grid gap-4 md:grid-cols-2">
        <Link
          href={`/shops/${shop_code}/dashboard/items`}
          className="p-4 border rounded hover:bg-gray-50"
        >
          商品管理
          <p className="text-xs text-gray-500 mt-1">※構築中</p>
        </Link>

        <Link
          href={`/shops/${shop_code}/dashboard/orders`}
          className="p-4 border rounded hover:bg-gray-50"
        >
          注文・配送管理
          <p className="text-xs text-gray-500 mt-1">
            ※手動(実装済み)→半自動（構築中）→自動配送管理(予定計画)
          </p>
        </Link>

        <Link
          href={`/shops/${shop_code}/dashboard/customers`}
          className="p-4 border rounded hover:bg-gray-50"
        >
          顧客管理
          <p className="text-xs text-gray-500 mt-1">※構築中</p>
        </Link>

        <Link
          href={`/shops/${shop_code}/dashboard/settings`}
          className="p-4 border rounded hover:bg-gray-50"
        >
          店舗設定
          <p className="text-xs text-gray-500 mt-1">※構築中</p>
        </Link>

        <div className="p-4 border rounded space-y-3 bg-yellow-50 border-yellow-300">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-lg">
              AtlaskernelSystem 解析レビュー（v3）
            </h2>
            <span className="text-xs px-2 py-1 rounded bg-yellow-500 text-white">
              テスト 管理者用
            </span>
          </div>

          <p className="text-sm text-gray-600">
            AI解析結果の確認・判断・再解析を行います。
          </p>

          <div className="flex flex-wrap gap-3 text-sm">
            <Link
              href={`/shops/${shop_code}/dashboard/atlas/requests`}
              className="text-blue-600 underline"
            >
              ▶ レビュー一覧
              <p className="text-xs text-gray-500">
                ※ 判断履歴は各レビュー詳細画面から確認できます
              </p>
            </Link>
          </div>
        </div>

        <div className="p-4 border rounded space-y-3 bg-sky-50 border-sky-300">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-lg">
              TrustLedger PaymentSystem 管理レビュー（v3）
            </h2>
            <span className="text-xs px-2 py-1 rounded bg-sky-600 text-white">
              Finance / Ledger
            </span>
          </div>

          <p className="text-sm text-gray-600">
            購入後の台帳・残高・ホールド・出金の監査と運用を行います。
          </p>

          <div className="flex flex-wrap gap-3 text-sm">
            <Link
              href={`/shops/${shop_code}/dashboard/trustledger`}
              className="text-blue-600 underline"
            >
              ▶ 管理レビュー一覧
              <p className="text-xs text-gray-500">
                ※PSP切り替え自動状況判断システム実装予定計画
              </p>
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
