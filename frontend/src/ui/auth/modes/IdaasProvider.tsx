"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import type { AuthContext, LoginPayload, ApiClient } from "@/ui/auth/contracts";
import type { AuthUser } from "@/domain/auth/AuthUser";
import { TokenStorage } from "@/infrastructure/auth/TokenStorage";
import { AuthContextCoreProvider } from "@/ui/auth/core/AuthContextCore";

/* =========================================================
   Bearer API Client
========================================================= */
function createBearerApiClient(): ApiClient {
  const base = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost";
  const apiBase = `${base.replace(/\/+$/, "")}/api`;

  const request = async <T,>(
    method: "GET" | "POST" | "PATCH" | "DELETE",
    url: string,
    body?: unknown,
  ): Promise<T> => {
    const fullUrl = url.startsWith("http")
      ? url
      : `${apiBase}${url.startsWith("/") ? "" : "/"}${url}`;

    const { accessToken } = TokenStorage.load();

    const headers: Record<string, string> = { Accept: "application/json" };
    if (accessToken) headers.Authorization = `Bearer ${accessToken}`;

    const isFormData =
      typeof FormData !== "undefined" && body instanceof FormData;

    // ✅ FormData のときは Content-Type を付けない（boundary を壊さない）
    if (method !== "GET" && !isFormData) {
      headers["Content-Type"] = "application/json";
    }

    const res = await fetch(fullUrl, {
      method,
      headers,
      body:
        method === "GET"
          ? undefined
          : body === undefined
            ? undefined
            : isFormData
              ? (body as FormData)
              : JSON.stringify(body),
      credentials: "omit",
      cache: "no-store",
    });

    if (!res.ok) {
      let msg = `Request failed: ${res.status}`;
      try {
        const ct = res.headers.get("content-type") || "";
        if (ct.includes("application/json")) {
          const j = await res.json().catch(() => ({}));
          msg = (j as any)?.message ?? msg;
        } else {
          const t = await res.text().catch(() => "");
          if (t) msg = t.slice(0, 300);
        }
      } catch {
        // ignore
      }
      const e: any = new Error(msg);
      e.status = res.status;
      throw e;
    }

    if (res.status === 204) return undefined as unknown as T;

    const ct = res.headers.get("content-type") || "";
    if (!ct.includes("application/json")) {
      const t = await res.text().catch(() => "");
      const e: any = new Error(`Non-JSON response: ${t.slice(0, 200)}`);
      e.status = 500;
      throw e;
    }

    return (await res.json()) as T;
  };

  return {
    get: <T,>(url: string) => request<T>("GET", url),
    post: <T,>(url: string, body?: unknown) => request<T>("POST", url, body),
    patch: <T,>(url: string, body?: unknown) => request<T>("PATCH", url, body),
    delete: <T,>(url: string) => request<T>("DELETE", url),
  };
}

/* =========================================================
   PKCE helpers
========================================================= */
function randomString(len = 64): string {
  const chars =
    "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~";
  const bytes = new Uint8Array(len);
  crypto.getRandomValues(bytes);
  let out = "";
  for (let i = 0; i < len; i++) out += chars[bytes[i] % chars.length];
  return out;
}

function base64UrlEncode(bytes: ArrayBuffer): string {
  const b = new Uint8Array(bytes);
  let s = "";
  for (let i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

async function sha256Base64Url(input: string): Promise<string> {
  const enc = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest("SHA-256", enc);
  return base64UrlEncode(digest);
}

/* =========================================================
   Keys
========================================================= */
const PKCE_VERIFIER_KEY = "auth0_pkce_verifier_v1";
const OIDC_STATE_KEY = "auth0_state_v1";
const OIDC_RETURN_TO_KEY = "auth0_return_to_v1";
const EXCHANGE_LOCK_KEY = "auth0_exchange_lock_v1";

const NAV_LOCK_KEY = "occore_nav_lock_v1";
const OWNER_REDIRECT_KEY = "occore_owner_shop_code_v1";
const JUST_LOGGED_IN_KEY = "occore_just_logged_in_v1";

// ✅ StrictMode 二重実行を完全に潰す “グローバルロック”
const GLOBAL_LOCK_KEY = "__occore_auth0_exchange_lock_v1";

function acquireGlobalLock(): boolean {
  const g = globalThis as any;
  if (g[GLOBAL_LOCK_KEY]) return false;
  g[GLOBAL_LOCK_KEY] = true;
  return true;
}
function releaseGlobalLock(): void {
  const g = globalThis as any;
  g[GLOBAL_LOCK_KEY] = false;
}

function safeReturnTo(raw: string | null | undefined): string {
  if (!raw) return "/";
  if (raw.startsWith("/")) return raw;
  return "/";
}

/* =========================================================
   Storage helpers
========================================================= */
function getSessionItem(key: string): string | null {
  try {
    return sessionStorage.getItem(key);
  } catch {
    return null;
  }
}
function setSessionItem(key: string, value: string): void {
  try {
    sessionStorage.setItem(key, value);
  } catch {
    // ignore
  }
}
function removeSessionItem(key: string): void {
  try {
    sessionStorage.removeItem(key);
  } catch {
    // ignore
  }
}

/**
 * ✅ PKCE/state/verifier は sessionStorage に統一（localStorage禁止）
 * - state_mismatch の主因（storage混在）を潰す
 */
function getPkceItem(key: string): string | null {
  return getSessionItem(key);
}
function setPkceItem(key: string, value: string): void {
  setSessionItem(key, value);
}
function removePkceItem(key: string): void {
  removeSessionItem(key);
}

/**
 * ✅ OIDC/PKCE 一時状態をまとめて消す（sessionStorageのみ）
 */
function clearOidcSessionState() {
  removeSessionItem(PKCE_VERIFIER_KEY);
  removeSessionItem(OIDC_STATE_KEY);
  removeSessionItem(OIDC_RETURN_TO_KEY);
  removeSessionItem(EXCHANGE_LOCK_KEY);
}

function navOnce(router: ReturnType<typeof useRouter>, to: string) {
  try {
    if (sessionStorage.getItem(NAV_LOCK_KEY) === "1") return;
    sessionStorage.setItem(NAV_LOCK_KEY, "1");
  } catch {
    // ignore
  }
  router.replace(to);
}

function clearNavLockSoon() {
  setTimeout(() => removeSessionItem(NAV_LOCK_KEY), 0);
}
function clearJustLoggedInSoon() {
  setTimeout(() => removeSessionItem(JUST_LOGGED_IN_KEY), 1500);
}

/* =========================================================
   Auth0 endpoints
========================================================= */
function buildAuth0Endpoints(domain: string) {
  const d = domain.replace(/^https?:\/\//, "").replace(/\/+$/, "");
  const origin = `https://${d}`;
  return {
    authorize: `${origin}/authorize`,
    token: `${origin}/oauth/token`,
    logout: `${origin}/v2/logout`,
  };
}

function normalizeScopes(scopes: string): string {
  return scopes.trim().replace(/\.$/, "").replace(/\s+/g, " ");
}

function pickOwnerShopCode(me: any): string | null {
  const roles = Array.isArray(me?.shop_roles) ? me.shop_roles : [];
  if (!roles.length) return null;
  const r0 = roles[0];
  if (r0?.role === "owner" && r0?.shop_code) return r0.shop_code;
  if (r0?.shop_code) return r0.shop_code;
  return null;
}

/* =========================================================
   Provider
========================================================= */
export default function IdaasProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();

  const [isLoading, setIsLoading] = useState(true);
  const [authReady, setAuthReady] = useState(false);
  const [user, setUser] = useState<AuthUser | null>(null);

  const apiClient = useMemo(() => createBearerApiClient(), []);

  const auth0Domain = process.env.NEXT_PUBLIC_AUTH0_DOMAIN ?? "";
  const clientId = process.env.NEXT_PUBLIC_AUTH0_CLIENT_ID ?? "";
  const audience = process.env.NEXT_PUBLIC_AUTH0_AUDIENCE ?? "";

  const redirectUri =
    process.env.NEXT_PUBLIC_OIDC_REDIRECT_URI ??
    "http://localhost/auth/callback";
  const postLogoutRedirectUri =
    process.env.NEXT_PUBLIC_OIDC_POST_LOGOUT_REDIRECT_URI ??
    "http://localhost/login";

  const scopes = normalizeScopes(
    process.env.NEXT_PUBLIC_OIDC_SCOPES ?? "openid profile email",
  );

  const endpoints = useMemo(
    () => buildAuth0Endpoints(auth0Domain),
    [auth0Domain],
  );

  const exchangeInFlight = useRef(false);

  const fetchMe = useCallback(async (): Promise<AuthUser | null> => {
    try {
      const u = await apiClient.get<AuthUser>("/me");
      setUser(u);
      return u;
    } catch (e: any) {
      if (e?.status === 401) TokenStorage.clear();
      setUser(null);
      return null;
    }
  }, [apiClient]);

  const refresh = useCallback(async () => {
    await fetchMe();
  }, [fetchMe]);

  useEffect(() => {
    (async () => {
      try {
        const code = searchParams.get("code");
        const state = searchParams.get("state");
        const error = searchParams.get("error");
        const errorDescription = searchParams.get("error_description");

        // Auth0側エラー
        if (error) {
          TokenStorage.clear();
          clearOidcSessionState();
          releaseGlobalLock();
          router.replace(
            `/login?oidc_error=${encodeURIComponent(error)}${
              errorDescription
                ? `&oidc_error_description=${encodeURIComponent(errorDescription)}`
                : ""
            }`,
          );
          return;
        }

        // ===== callback =====
        if (code) {
          // ✅ global lock（StrictMode 二重実行を殺す）
          if (!acquireGlobalLock()) return;

          // ✅ session lock（同一tab内の二重実行も抑止）
          if (getSessionItem(EXCHANGE_LOCK_KEY) === "1") return;
          setSessionItem(EXCHANGE_LOCK_KEY, "1");

          if (exchangeInFlight.current) return;
          exchangeInFlight.current = true;

          if (!auth0Domain || !clientId || !audience) {
            TokenStorage.clear();
            clearOidcSessionState();
            releaseGlobalLock();
            router.replace("/login?oidc_error=env_missing");
            return;
          }

          // ✅ state/verifier は sessionStorage（統一）
          const expectedState = getPkceItem(OIDC_STATE_KEY);
          if (!expectedState || state !== expectedState) {
            TokenStorage.clear();
            clearOidcSessionState();
            releaseGlobalLock();
            router.replace("/login?oidc_error=state_mismatch");
            return;
          }

          const verifier = getPkceItem(PKCE_VERIFIER_KEY);
          if (!verifier) {
            TokenStorage.clear();
            clearOidcSessionState();
            releaseGlobalLock();
            router.replace("/login?oidc_error=missing_verifier");
            return;
          }

          const body = new URLSearchParams();
          body.set("grant_type", "authorization_code");
          body.set("client_id", clientId);
          body.set("code", code);
          body.set("redirect_uri", redirectUri);
          body.set("code_verifier", verifier);
          body.set("audience", audience);

          const res = await fetch(endpoints.token, {
            method: "POST",
            headers: {
              "Content-Type": "application/x-www-form-urlencoded",
              Accept: "application/json",
            },
            body: body.toString(),
            cache: "no-store",
          });

          if (!res.ok) {
            const text = await res.text().catch(() => "");
            TokenStorage.clear();
            clearOidcSessionState();
            releaseGlobalLock();
            router.replace(
              `/login?oidc_error=token_exchange_failed&status=${res.status}${
                text ? `&detail=${encodeURIComponent(text.slice(0, 200))}` : ""
              }`,
            );
            return;
          }

          const json = (await res.json().catch(() => ({}))) as any;
          const accessToken =
            typeof json?.access_token === "string" ? json.access_token : "";
          const refreshToken =
            typeof json?.refresh_token === "string" ? json.refresh_token : "";

          if (!accessToken) {
            TokenStorage.clear();
            clearOidcSessionState();
            releaseGlobalLock();
            router.replace("/login?oidc_error=missing_access_token");
            return;
          }

          TokenStorage.save({ accessToken, refreshToken });

          // ✅ me を取得
          const me = await fetchMe();

          // ✅ returnTo 最優先（verify→profile を保証）
          const returnToRaw = getSessionItem(OIDC_RETURN_TO_KEY);
          let returnTo = safeReturnTo(returnToRaw);

          // ✅ 追加：profile_completed 済みなら profile への強制 returnTo を無効化
          const completed =
            !!(me as any)?.profile_completed || !!(me as any)?.profileCompleted;

          if (completed && returnTo === "/mypage/profile") {
            returnTo = "/";
          }

          // ✅ 成功したので最後に “一括クリア”
          clearOidcSessionState();
          releaseGlobalLock();

          if (returnTo && returnTo !== "/") {
            navOnce(router, returnTo);
            clearNavLockSoon();
            clearJustLoggedInSoon();
            return;
          }

          // returnTo が無い場合のみ、従来ルール
          const shopCode = pickOwnerShopCode(me as any);
          if (shopCode) {
            setSessionItem(JUST_LOGGED_IN_KEY, "1");
            setSessionItem(OWNER_REDIRECT_KEY, shopCode);
            window.location.assign(`/shops/${shopCode}/dashboard`);
            return;
          }

          navOnce(router, "/");
          clearNavLockSoon();
          clearJustLoggedInSoon();
          return;
        }

        // ===== normal init =====
        const { accessToken } = TokenStorage.load();
        if (accessToken) {
          await fetchMe();
        }
      } finally {
        setIsLoading(false);
        setAuthReady(true);
      }
    })();
  }, [
    searchParams,
    router,
    auth0Domain,
    clientId,
    audience,
    redirectUri,
    endpoints.token,
    fetchMe,
  ]);

  /**
   * ✅ AuthContext の union に合わせた login
   * - IdaasProvider は kind=idaas 以外を拒否
   */
  const login = useCallback(
    async (payload: LoginPayload) => {
      // PasswordログインはIdaaSでは扱わない（呼ばれたら “開始だけ” にしても良い）
      // ここでは「間違って呼ばれても落とさない」方針にする。
      const returnToFromPayload =
        payload.type === "oidc" ? payload.returnTo : undefined;

      if (!auth0Domain || !clientId) {
        throw new Error(
          "Auth0 env missing: NEXT_PUBLIC_AUTH0_DOMAIN / NEXT_PUBLIC_AUTH0_CLIENT_ID",
        );
      }
      if (!audience) {
        throw new Error("Auth0 env missing: NEXT_PUBLIC_AUTH0_AUDIENCE");
      }

      const verifier = randomString(64);
      const challenge = await sha256Base64Url(verifier);

      try {
        // ログイン開始：古いロック/フラグを掃除
        removeSessionItem(EXCHANGE_LOCK_KEY);
        releaseGlobalLock();
        removeSessionItem(NAV_LOCK_KEY);
        removeSessionItem(OWNER_REDIRECT_KEY);
        removeSessionItem(JUST_LOGGED_IN_KEY);

        // ✅ OIDC/PKCE残骸も掃除（中途半端な前回セッション対策）
        removeSessionItem(OIDC_RETURN_TO_KEY);
        removeSessionItem(OIDC_STATE_KEY);
        removeSessionItem(PKCE_VERIFIER_KEY);

        // ✅ PKCE (sessionStorage)
        setPkceItem(PKCE_VERIFIER_KEY, verifier);

        const s = randomString(32);
        setPkceItem(OIDC_STATE_KEY, s);

        // ✅ returnTo をセッションへ
        const injected = safeReturnTo(returnToFromPayload ?? null);
        const currentPath =
          typeof window !== "undefined"
            ? `${window.location.pathname}${window.location.search}`
            : "/";
        const fallback = currentPath.startsWith("/login") ? "/" : currentPath;

        const returnTo = injected !== "/" ? injected : fallback;
        setSessionItem(OIDC_RETURN_TO_KEY, returnTo);

        const params = new URLSearchParams();
        params.set("response_type", "code");
        params.set("client_id", clientId);
        params.set("redirect_uri", redirectUri);
        params.set("scope", scopes);
        params.set("audience", audience);
        params.set("code_challenge", challenge);
        params.set("code_challenge_method", "S256");
        params.set("state", s);

        window.location.assign(`${endpoints.authorize}?${params.toString()}`);
      } catch {
        TokenStorage.clear();
        clearOidcSessionState();
        releaseGlobalLock();
        throw new Error("auth0_login_start_failed");
      }
    },
    [auth0Domain, clientId, audience, redirectUri, scopes, endpoints.authorize],
  );

  const logout = useCallback(async () => {
    TokenStorage.clear();
    clearOidcSessionState();
    releaseGlobalLock();
    setUser(null);

    removeSessionItem(NAV_LOCK_KEY);
    removeSessionItem(OWNER_REDIRECT_KEY);
    removeSessionItem(JUST_LOGGED_IN_KEY);

    if (!auth0Domain || !clientId) {
      router.replace("/login");
      return;
    }

    const params = new URLSearchParams();
    params.set("client_id", clientId);
    params.set("returnTo", postLogoutRedirectUri);

    window.location.assign(`${endpoints.logout}?${params.toString()}`);
  }, [auth0Domain, clientId, endpoints.logout, postLogoutRedirectUri, router]);

  const value: AuthContext = useMemo(
    () => ({
      isLoading,
      authReady,
      isAuthenticated: !!user,
      user,
      apiClient,
      login, // ← login(payload: LoginPayload)
      logout,
      refresh,
    }),
    [isLoading, authReady, user, apiClient, login, logout, refresh],
  );

  return (
    <AuthContextCoreProvider value={value}>{children}</AuthContextCoreProvider>
  );
}
