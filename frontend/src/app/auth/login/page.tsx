"use client";

import { useEffect } from "react";
import { useSearchParams } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

export default function AuthLoginPage() {
  const sp = useSearchParams();
  const { login } = useAuth();

  useEffect(() => {
    const returnTo = sp.get("returnTo") ?? "/";

    login({ kind: "idaas", returnTo }).catch(() => {
      // 失敗時は login 側で /login?oidc_error=... へ飛ばす想定
    });
  }, [sp, login]);

  return (
    <div style={{ padding: 24 }}>
      <h1>Signing in…</h1>
      <p>認証を開始しています。</p>
    </div>
  );
}
