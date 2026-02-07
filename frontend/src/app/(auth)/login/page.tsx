"use client";

import React, { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

export default function LoginPage() {
  const router = useRouter();
  const { login, isLoading, apiClient, refresh } = useAuth() as any;

  const mode = process.env.NEXT_PUBLIC_AUTH_MODE ?? "sanctum";
  const isIdaas = mode === "idaas";
  const isDev = process.env.NODE_ENV !== "production";

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [apiError, setApiError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const adminBase =
    process.env.NEXT_PUBLIC_ADMIN_RAILS_BASE_URL ?? "http://localhost:3001";
  const adminDashboardUrl = `${adminBase}/admin/dashboard`;

  const modeLabel = useMemo(() => {
    if (mode === "idaas") return "IdaaS (Auth0 PKCE)";
    if (mode === "firebase_jwt") return "Firebase JWT";
    return "Sanctum (Stateful)";
  }, [mode]);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setApiError("");
    setIsSubmitting(true);

    try {
      // Idaas(Auth0) は PKCE リダイレクトが本体なので、email/passwordは使わない
      if (isIdaas) {
        await login({ type: "oidc", returnTo: "/" });
        return;
      }

      // Sanctum / firebase_jwt
      await login({ kind: "password", email, password });

      // AuthProvider 側で user 確定を揃える（互換alias）
      if (typeof refresh === "function") {
        await refresh();
      }

      const me = await apiClient.get("/me");

      if (!me?.email_verified_at) {
        router.replace("/email/verify");
        return;
      }

      if (me?.profile_completed === false) {
        router.replace("/mypage/profile");
        return;
      }

      const shopRoles = Array.isArray(me?.shop_roles) ? me.shop_roles : [];
      const primary = shopRoles[0];
      if (primary?.shop_code) {
        router.replace(`/shops/${primary.shop_code}/dashboard`);
        return;
      }

      router.replace("/");
    } catch (e: any) {
      setApiError(
        isIdaas ? "SSOログインの開始に失敗しました" : "ログインに失敗しました",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="w-full max-w-xl p-8 mx-auto mt-20 bg-white rounded-xl shadow-xl">
      <h2 className="mb-6 text-3xl font-bold text-center border-b pb-3">
        ログイン
      </h2>

      {isDev && (
        <div className="mb-6 p-3 rounded border bg-gray-50 text-sm">
          <div className="font-semibold">開発用：認証モード</div>
          <div className="mt-1">
            現在：<span className="font-mono">{mode}</span>（{modeLabel}）
          </div>

          <div className="mt-2 text-xs text-gray-600">
            切替は .env の{" "}
            <span className="font-mono">NEXT_PUBLIC_AUTH_MODE</span> を変更して
            Next.js を再起動してください。
          </div>

          <div className="mt-3 flex flex-col gap-1">
            <span className="text-xs text-gray-600">利用する導線：</span>
            <ul className="list-disc ml-5 text-xs text-gray-700 space-y-1">
              <li>
                <span className="font-mono">idaas</span>：この画面の
                「SSOでログイン」ボタン（Auth0 PKCEリダイレクト）
              </li>
              <li>
                <span className="font-mono">sanctum</span> /{" "}
                <span className="font-mono">firebase_jwt</span>
                ：下のメール/パスワードフォーム
              </li>
            </ul>

            <div className="mt-2 flex flex-wrap gap-2">
              <a className="text-xs underline text-gray-700" href="/login">
                /login を再表示
              </a>
              <a
                className="text-xs underline text-gray-700"
                href="/auth/callback"
              >
                /auth/callback（PKCE受口）
              </a>
            </div>
          </div>
        </div>
      )}

      {apiError && (
        <div className="p-3 mb-4 text-sm text-red-700 bg-red-100 rounded">
          {apiError}
        </div>
      )}

      {isIdaas ? (
        <form onSubmit={handleSubmit} className="space-y-6">
          <button
            type="submit"
            disabled={isSubmitting || isLoading}
            className="w-full py-3 bg-black text-white rounded"
          >
            {isSubmitting ? "リダイレクト中..." : "SSOでログイン（Auth0）"}
          </button>

          <p className="text-xs text-gray-600">
            SSOログインはメール/パスワード入力ではなく、Auth0へリダイレクトします。
          </p>
        </form>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label className="block mb-1 text-sm">メールアドレス</label>
            <input
              type="email"
              className="w-full px-4 py-2 border rounded"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              disabled={isSubmitting || isLoading}
            />
          </div>

          <div>
            <label className="block mb-1 text-sm">パスワード</label>
            <input
              type="password"
              className="w-full px-4 py-2 border rounded"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              disabled={isSubmitting || isLoading}
            />
          </div>

          <button
            type="submit"
            disabled={isSubmitting || isLoading}
            className="w-full py-3 bg-red-600 text-white rounded"
          >
            {isSubmitting ? "ログイン中..." : "ログインする"}
          </button>
        </form>
      )}

      <div className="mt-6 text-center space-y-2">
        <Link href="/register" className="text-blue-500 text-sm block">
          会員登録はこちら
        </Link>

        <a
          href={adminDashboardUrl}
          className="text-gray-600 text-xs underline block"
          target="_blank"
          rel="noreferrer"
        >
          管理者/開発者コンソールへ（admin/dashboard）（開発用）
        </a>
      </div>
    </div>
  );
}
