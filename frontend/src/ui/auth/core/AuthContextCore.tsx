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

function isSensitiveAuthFlowPath(): boolean {
  if (typeof window === "undefined") return false;
  const p = window.location.pathname || "";

  // ✅ 認証コールバック・メール検証・ログイン導線では副作用禁止
  if (p.startsWith("/auth/callback")) return true;
  if (p.startsWith("/email/verify")) return true;
  if (p.startsWith("/login")) return true;

  return false;
}

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

  const authReady = !!(value as any).authReady;
  const isAuthenticated = !!(value as any).isAuthenticated;

  const userEmail: string | null =
    ((value as any)?.user?.email as string | undefined) ?? null;

  const prev = useRef<{
    ready: boolean;
    authed: boolean;
    email: string | null;
  }>({ ready: false, authed: false, email: null });

  useEffect(() => {
    const was = prev.current;

    const becameAuthed =
      authReady &&
      isAuthenticated &&
      !!userEmail &&
      (!was.ready || !was.authed || was.email !== userEmail);

    // ✅ callback/verify/login では last_login_at を絶対触らない
    if (becameAuthed && !isSensitiveAuthFlowPath()) {
      touchLastLoginAt();
    }

    // ✅ user が確定したら pending_* を掃除（OK）
    if (authReady && isAuthenticated && userEmail) {
      try {
        localStorage.removeItem("pending_email");
        localStorage.removeItem("pending_display_name");
        localStorage.removeItem("pending_email_expires_at");
      } catch {}
    }

    prev.current = {
      ready: authReady,
      authed: isAuthenticated,
      email: userEmail,
    };
  }, [authReady, isAuthenticated, userEmail]);

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
