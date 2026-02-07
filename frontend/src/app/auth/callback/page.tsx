"use client";

import { useEffect, useMemo, useRef } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

type Screen = "login" | "verify" | "verify_done";

export default function AuthCallbackPage() {
  const sp = useSearchParams();
  const router = useRouter();
  const { login } = useAuth();

  const screen: Screen = (sp.get("screen") as Screen) ?? "login";
  const returnTo = sp.get("returnTo") ?? "/";

  // ✅ Auth0 callback 判定
  const code = sp.get("code");
  const error = sp.get("error");

  const isVerify = screen === "verify" || screen === "verify_done";
  const startedRef = useRef(false);

  useEffect(() => {
    // ✅ verify画面では自動開始しない（ボタンで開始）
    if (isVerify) return;

    // ✅ Auth0 から戻った callback（code/errorあり）では “開始しない”
    //    → IdaasProvider が token exchange を行う
    if (code || error) return;

    if (screen !== "login") return;
    if (startedRef.current) return;
    startedRef.current = true;

    login({ type: "oidc", returnTo }).catch(() => {
      router.replace(`/login?oidc_error=1`);
    });
  }, [screen, returnTo, isVerify, code, error, login, router]);

  const onContinue = async () => {
    // ✅ ここは router.replace ではなく login() 直呼びが一番確実
    await login({ type: "oidc", returnTo });
  };

  return (
    <div style={{ padding: 24, maxWidth: 560 }}>
      <h1>
        {isVerify
          ? screen === "verify_done"
            ? "メール認証が完了しました"
            : "確認メールを送信しました"
          : "Signing in…"}
      </h1>
      <p>
        {isVerify
          ? screen === "verify_done"
            ? "メール認証が完了しました。「続ける」を押してログインを開始してください。"
            : "確認メールを送信しました。メール内リンクで認証後、「続ける」を押してください。"
          : "SSO認証を開始しています。"}
      </p>

      {isVerify ? (
        <button
          onClick={onContinue}
          style={{
            padding: "10px 14px",
            border: "1px solid #ccc",
            borderRadius: 8,
            cursor: "pointer",
            marginTop: 16,
          }}
        >
          続ける
        </button>
      ) : null}
    </div>
  );
}
