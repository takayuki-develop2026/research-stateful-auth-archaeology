"use client";

import { useEffect, useMemo, useRef } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

type Screen = "login" | "verify" | "verify_done";

function d(...args: any[]) {
  // eslint-disable-next-line no-console
  console.log("[AuthCallback]", ...args);
}

export default function AuthCallbackPage() {
  const sp = useSearchParams();
  const router = useRouter();
  const { login } = useAuth();

  const screen = (sp.get("screen") as Screen) ?? "login";
  const returnTo = sp.get("returnTo") ?? "/";

  const code = sp.get("code");
  const error = sp.get("error");

  // verify系は「ボタンで開始」
  const isVerify = screen === "verify" || screen === "verify_done";

  // UI文字列は state じゃなく「条件から導出」
  const title = useMemo(() => {
    if (isVerify) {
      return screen === "verify_done"
        ? "メール認証が完了しました"
        : "確認メールを送信しました";
    }
    return "Signing in…";
  }, [isVerify, screen]);

  const message = useMemo(() => {
    if (isVerify) {
      return screen === "verify_done"
        ? "メール認証が完了しました。「続ける」を押してログインを開始してください。"
        : "確認メールを送信しました。メール内リンクで認証後、「続ける」を押してください。";
    }
    // Auth0 から code 付きで戻った直後は「交換中」っぽく見せる
    if (code) return "認証情報を受け取りました。トークン交換中…";
    if (error) return "SSOでエラーが発生しました。ログイン画面に戻ります…";
    return "SSO認証を開始しています。";
  }, [isVerify, screen, code, error]);

  const startedRef = useRef(false);

  useEffect(() => {
    d("mounted", { screen, returnTo, hasCode: !!code, error });

    // verify画面は自動開始しない
    if (isVerify) return;

    // Auth0 から戻った callback（code/errorあり）ではここで開始しない
    // → IdaasProvider 側が token exchange / refresh をやる想定
    if (code || error) {
      if (error) {
        // error の場合だけログインへ戻す（任意）
        router.replace(`/login?oidc_error=1`);
      }
      return;
    }

    // login screen のみ自動開始
    if (screen !== "login") return;

    // StrictMode二重実行対策
    if (startedRef.current) return;
    startedRef.current = true;

    performance.mark("authcb:login:start");
    d("auto login(oidc) start", { returnTo });

    // setStateはしない。ログだけ。
    login({ type: "oidc", returnTo })
      .then(() => {
        performance.mark("authcb:login:called");
        d("login(oidc) called");
      })
      .catch((e: any) => {
        console.error("🔥DEBUG login(oidc) failed", e);
        router.replace(`/login?oidc_error=1`);
      });
  }, [screen, returnTo, isVerify, code, error, login, router]);

  const onContinue = async () => {
    try {
      sessionStorage.setItem("occore_return_to_v1", returnTo); // ✅ 保存
      console.log("[AuthCallback] saved returnTo", returnTo);

      await login({ type: "oidc", returnTo }); // login側でも保存するなら二重でもOK
    } catch (e) {
      console.error(e);
      router.replace(`/login?oidc_error=1`);
    }
  };

  return (
    <div style={{ padding: 24, maxWidth: 560 }}>
      <h1>{title}</h1>
      <p>{message}</p>

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

      {/* デバッグ表示（任意） */}
      <pre style={{ marginTop: 16, fontSize: 12, opacity: 0.7 }}>
        {JSON.stringify({ screen, returnTo, code: !!code, error }, null, 2)}
      </pre>
    </div>
  );
}
