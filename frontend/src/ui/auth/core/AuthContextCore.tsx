"use client";

import React, {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from "react";
import type { AuthContext } from "@/ui/auth/contracts";

const LAST_LOGIN_AT_KEY = "last_login_at";
const LAST_LOGIN_AT_EVENT = "occ:last_login_at_changed";

function touchLastLoginAt(): void {
  try {
    localStorage.setItem(LAST_LOGIN_AT_KEY, new Date().toISOString());
    window.dispatchEvent(new Event(LAST_LOGIN_AT_EVENT));
  } catch {}
}

function clearLastLoginAt(): void {
  try {
    localStorage.removeItem(LAST_LOGIN_AT_KEY);
    window.dispatchEvent(new Event(LAST_LOGIN_AT_EVENT));
  } catch {}
}

export const AuthCtx = createContext<AuthContext | null>(null);

export function useAuth(): AuthContext {
  const ctx = useContext(AuthCtx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function AuthContextCoreProvider(props: {
  children: React.ReactNode;
  value: AuthContext;
}) {
  const { children, value } = props;

  // contracts のキーを正として扱う（あなたは authReady を使っている）
  const authReady = !!(value as any).authReady;
  const isAuthenticated = !!(value as any).isAuthenticated;

  // ✅ “認証が確定して authenticated になった瞬間” を setState無しで検知
  const prev = useRef<{ ready: boolean; authed: boolean }>({
    ready: false,
    authed: false,
  });

  useEffect(() => {
    const wasReady = prev.current.ready;
    const wasAuthed = prev.current.authed;

    if (authReady && isAuthenticated && (!wasReady || !wasAuthed)) {
      touchLastLoginAt();
    }

    prev.current = { ready: authReady, authed: isAuthenticated };
  }, [authReady, isAuthenticated]);

  // ✅ logout だけ共通処理を差し込む（その他は素通し）
  const wrapped = useMemo<AuthContext>(() => {
    const base: any = value;

    return {
      ...base,
      logout: async () => {
        await base.logout();
        clearLastLoginAt();
      },
    } as AuthContext;
  }, [value]);

  return <AuthCtx.Provider value={wrapped}>{children}</AuthCtx.Provider>;
}
