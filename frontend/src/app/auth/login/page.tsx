"use client";

import { useEffect, useMemo, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

type Screen = "login" | "verify";

export default function AuthLoginPage() {
  const sp = useSearchParams();
  const router = useRouter();
  const { login } = useAuth();

  const screen: Screen = (sp.get("screen") as Screen) ?? "login";
  const returnTo = sp.get("returnTo") ?? "/";

  // 1回目/2回目表示用（sessionStorage）
  const [sendCount, setSendCount] = useState<number>(0);

  useEffect(() => {
    if (screen !== "verify") return;

    // verify画面に来た回数を数える（= 送信後に戻ってきた回数として扱う）
    const key = "verify_email_sent_count";
    const current = Number(sessionStorage.getItem(key) ?? "0");
    const next = Number.isFinite(current) ? current + 1 : 1;
    sessionStorage.setItem(key, String(next));
    setSendCount(next);
  }, [screen]);

  // ✅ login画面のときだけ自動でOIDC開始（いまの挙動を維持）
  useEffect(() => {
    if (screen === "verify") return;

    login({ type: "oidc", returnTo }).catch(() => {
      // IdaasProvider.login 側で cleanup 済み想定
      // 必要なら router.replace("/login?oidc_error=1") 等も可
    });
  }, [screen, returnTo, login]);

  const title = useMemo(() => {
    if (screen !== "verify") return "Signing in…";
    return sendCount >= 2
      ? "Verification email re-sent"
      : "Verification email sent";
  }, [screen, sendCount]);

  const message = useMemo(() => {
    if (screen !== "verify") return "SSO認証を開始しています。";

    if (sendCount >= 2) {
      return "確認メールを再送しました。受信箱に無い場合は迷惑メールも確認してください。メール内のリンクを開いて認証後、「認証したので続ける」を押してください。";
    }
    return "確認メールを送信しました。メール内のリンクを開いて認証してください。認証後、この画面に戻って「認証したので続ける」を押してください。";
  }, [screen, sendCount]);

  const onContinue = async () => {
    // 認証完了後の再ログイン（= 通常のOIDC開始）
    await login({ type: "oidc", returnTo });
  };

  const onBackToLogin = () => {
    // verify表示をやめて通常ログインへ（自動 login が走る）
    router.replace(`/auth/login?returnTo=${encodeURIComponent(returnTo)}`);
  };

  return (
    <div style={{ padding: 24, maxWidth: 560 }}>
      <h1>{title}</h1>
      <p>{message}</p>

      {screen === "verify" ? (
        <div style={{ marginTop: 16, display: "flex", gap: 12 }}>
          <button
            onClick={onContinue}
            style={{
              padding: "10px 14px",
              border: "1px solid #ccc",
              borderRadius: 8,
              cursor: "pointer",
            }}
          >
            認証したので続ける
          </button>

          <button
            onClick={onBackToLogin}
            style={{
              padding: "10px 14px",
              border: "1px solid #eee",
              borderRadius: 8,
              cursor: "pointer",
              background: "#fafafa",
            }}
          >
            通常ログインに戻る
          </button>
        </div>
      ) : null}
    </div>
  );
}
