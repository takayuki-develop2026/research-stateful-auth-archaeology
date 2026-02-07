"use client";

import { useEffect } from "react";
import { useSearchParams } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

export default function AuthLoginPage() {
  const sp = useSearchParams();
  const { login } = useAuth();

  useEffect(() => {
    const returnTo = sp.get("returnTo") ?? "/";

    // IdaasProvider 側は payload.type === "oidc" を見ている想定
    login({ type: "oidc", returnTo }).catch(() => {
      // 失敗時は IdaasProvider.login 内で state/lock cleanup 済みで例外になる
      // 必要ならここで /login?oidc_error=... に飛ばしても良い
    });
  }, [sp, login]);

  return (
    <div style={{ padding: 24 }}>
      <h1>Signing in…</h1>
      <p>SSO認証を開始しています。</p>
    </div>
  );
}
