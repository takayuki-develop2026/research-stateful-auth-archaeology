"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

type ShopOutput = {
  id: number;
  shop_code: string;
  name: string;
  status: string;
};

type ShopsMeResponse =
  | { shop: ShopOutput }
  | { shops: ShopOutput[] }
  | ShopOutput
  | ShopOutput[];

// function getCookieValue(name: string): string | null {
//   if (typeof document === "undefined") return null;
//   const m = document.cookie.match(new RegExp("(^| )" + name + "=([^;]+)"));
//   return m ? m[2] : null;
// }
// function getXsrfToken(): string | null {
//   const raw = getCookieValue("XSRF-TOKEN");
//   if (!raw) return null;
//   try {
//     return decodeURIComponent(raw);
//   } catch {
//     return raw;
//   }
// }



export default function ShopDashboardPage() {
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

  // 追加：表示用
  const [shopName, setShopName] = useState<string | null>(null);
const [loginAt] = useState<string | null>(() => {
  try {
    return localStorage.getItem("last_login_at");
  } catch {
    return null;
  }
});

  // shop の自分情報（/api/shops/me）から shop_name を取得
  useEffect(() => {
    if (!authReady || isAuthLoading) return;
    if (!isAuthenticated) return;
    if (!shop_code) return;

    (apiClient.get("/shops/me") as Promise<ShopsMeResponse>)
      .then((res) => {
        let shops: ShopOutput[] = [];

        if (Array.isArray(res)) {
          shops = res as any;
        } else if ((res as any)?.shops && Array.isArray((res as any).shops)) {
          shops = (res as any).shops;
        } else if ((res as any)?.shop) {
          shops = [(res as any).shop];
        } else if ((res as any)?.shop_code) {
          shops = [res as any];
        }

        const hit = shops.find((s) => s.shop_code === shop_code);
        setShopName(hit?.name ?? null);
      })
      .catch(() => setShopName(null));
  }, [authReady, isAuthLoading, isAuthenticated, shop_code, apiClient]);

  if (!authReady || isAuthLoading) {
    return <div className="p-6">読み込み中...</div>;
  }

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

  if (!isShopStaff) {
    return <div className="p-6">アクセス権限がありません。</div>;
  }

  return (
    <div className="p-6 space-y-4">
      <Link href={`/shops/${shop_code}`} className="text-blue-600 underline">
        ← 店舗トップへ戻る
      </Link>

      <div className="space-y-1">
        <h1 className="text-3xl font-bold">店舗ダッシュボード</h1>

        <p className="text-sm text-gray-600">
          店舗名: {shopName ?? shop_code} / ユーザー名:{" "}
          {user?.display_name ?? user?.email ?? "-"}（{roleInShop ?? "-"}）
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
          <p className="text-xs text-gray-500 mt-1">※手動→半自動（構築中）→自動配送管理予定計画</p>
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
              <p className="text-xs text-gray-500">※Stripe以外のPSP切り替え実装予定計画</p>
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
