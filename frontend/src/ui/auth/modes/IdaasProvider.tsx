"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import type { AuthContext, LoginPayload, ApiClient } from "@/ui/auth/contracts";
import type { AuthUser } from "@/domain/auth/AuthUser";
import { TokenStorage } from "@/infrastructure/auth/TokenStorage";
import { AuthContextCoreProvider } from "@/ui/auth/core/AuthContextCore";

/* =========================================================
   Logger
========================================================= */
function makeRunId(prefix = "oidc") {
  return `${prefix}-${Date.now().toString(36)}-${Math.random()
    .toString(36)
    .slice(2, 8)}`;
}
function authLog(runId: string, label: string, data?: any) {
  console.log(`[Idaas][${runId}] ${label}`, data ?? "");
}
function authErr(runId: string, label: string, data?: any) {
  console.error(`[Idaas][${runId}] ${label}`, data ?? "");
}
function maskToken(t?: string) {
  if (!t) return null;
  return `${t.slice(0, 10)}…${t.slice(-6)}`;
}

/* =========================================================
   Bearer API Client
========================================================= */
function normalizeBool(v: any): boolean {
  if (v === true) return true;
  if (v === false) return false;
  if (v === 1 || v === "1") return true;
  if (v === 0 || v === "0") return false;
  return false;
}

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
      } catch {}
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
   Keys（✅固定）
========================================================= */
const PKCE_VERIFIER_KEY = "auth0_pkce_verifier_v1";
const OIDC_STATE_KEY = "auth0_state_v1";

// ✅ SINGLE SOURCE OF TRUTH
const RETURN_TO_KEY = "auth0_return_to_v1";

const EXCHANGE_LOCK_KEY = "auth0_exchange_lock_v1";
const NAV_LOCK_KEY = "occore_nav_lock_v1";
const OWNER_REDIRECT_KEY = "occore_owner_shop_code_v1";
const JUST_LOGGED_IN_KEY = "occore_just_logged_in_v1";

// StrictMode 二重実行を潰す “グローバルロック”
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
  } catch {}
}
function removeSessionItem(key: string): void {
  try {
    sessionStorage.removeItem(key);
  } catch {}
}

/**
 * ✅ callback後に消すもの（returnTo/lockは「遷移発火後」に消す）
 */
function clearOidcSessionStateAfterCallback() {
  removeSessionItem(PKCE_VERIFIER_KEY);
  removeSessionItem(OIDC_STATE_KEY);
  // EXCHANGE_LOCK_KEY は残す（StrictMode/二重処理防止）。次の login() が必ず消す。
  // RETURN_TO_KEY は nav 発火後に消す。
}

const NAV_GLOBAL_LOCK = "__occore_nav_lock_until_v1";

function navOnce(router: ReturnType<typeof useRouter>, to: string) {
  const g = globalThis as any;
  const now = Date.now();
  const until = typeof g[NAV_GLOBAL_LOCK] === "number" ? g[NAV_GLOBAL_LOCK] : 0;

  // まだロック中なら二重遷移を抑止
  if (now < until) return;

  // 2秒だけロック（十分）
  g[NAV_GLOBAL_LOCK] = now + 2000;

  router.replace(to);
}
function clearNavLockSoon() {
  setTimeout(() => removeSessionItem(NAV_LOCK_KEY), 0);
}
function clearJustLoggedInSoon() {
  setTimeout(() => removeSessionItem(JUST_LOGGED_IN_KEY), 1500);
}
function clearReturnToSoon() {
  setTimeout(() => removeSessionItem(RETURN_TO_KEY), 0);
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
   Retry helpers
========================================================= */
async function sleep(ms: number) {
  await new Promise((r) => setTimeout(r, ms));
}

function isHoldReturnTo(path: string): boolean {
  return path.startsWith("/auth/callback?") && path.includes("p=");
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

  const runIdRef = useRef<string>(makeRunId("init"));
  const exchangeInFlight = useRef(false);

  /**
   * ✅ fetchMe を「normal / callback」で分離
   * - callback中は 401/403 を “即Token破棄”しない（揺れ対策）
   */
  const fetchMeRaw = useCallback(
    async (
      runId: string,
    ): Promise<
      | { ok: true; me: AuthUser }
      | { ok: false; status?: number; message?: string }
    > => {
      try {
        authLog(runId, "me:request", {});
        const u = await apiClient.get<AuthUser>("/me");
        authLog(runId, "me:response", {
          id: (u as any)?.id ?? null,
          email: (u as any)?.email ?? null,
          email_verified_at: (u as any)?.email_verified_at ?? null,
          profile_completed_raw: (u as any)?.profile_completed ?? null,
          profile_completed_norm: normalizeBool((u as any)?.profile_completed),
          shop_roles: (u as any)?.shop_roles ?? null,
        });
        return { ok: true, me: u };
      } catch (e: any) {
        authErr(runId, "me:error", {
          status: e?.status,
          message: String(e?.message ?? e),
        });
        return {
          ok: false,
          status: e?.status,
          message: String(e?.message ?? e),
        };
      }
    },
    [apiClient],
  );

  const fetchMeNormalInit = useCallback(
    async (runId?: string) => {
      const rid = runId ?? runIdRef.current;
      const r = await fetchMeRaw(rid);
      if (r.ok) {
        setUser(r.me);
        return r.me;
      }

      // normal init: 401/403 は token を捨てる
      if (r.status === 401 || r.status === 403) {
        TokenStorage.clear();
      }
      setUser(null);
      return null;
    },
    [fetchMeRaw],
  );

  const fetchMeWithRetryAfterExchange = useCallback(
    async (runId: string, tries = 3) => {
      for (let i = 1; i <= tries; i++) {
        const r = await fetchMeRaw(runId);

        if (r.ok) {
          setUser(r.me);
          return { ok: true as const, me: r.me };
        }

        // 401/403 でも「即死」させない。短い遅延で再試行（token書き込み/反映の競合対策）
        if ((r.status === 401 || r.status === 403) && i < tries) {
          authLog(runId, "me:retry_wait_auth", {
            attempt: i,
            nextInMs: 150 * i,
          });
          await sleep(150 * i);
          continue;
        }

        // それ以外や最終回は失敗として返す
        return {
          ok: false as const,
          status: r.status ?? 0,
          kind:
            r.status === 401 || r.status === 403
              ? ("unauthorized" as const)
              : ("transient" as const),
        };
      }
      return { ok: false as const, status: 0, kind: "transient" as const };
    },
    [fetchMeRaw],
  );

  const refresh = useCallback(async () => {
    await fetchMeNormalInit(makeRunId("refresh"));
  }, [fetchMeNormalInit]);

  const handleCallback = useCallback(
    async (args: { code: string; state: string | null }) => {
      const runId = makeRunId("callback");
      runIdRef.current = runId;

      const { code, state } = args;

      const returnToAtEnter = getSessionItem(RETURN_TO_KEY);
      authLog(runId, "callback:enter", {
        codePrefix: code.slice(0, 6),
        statePrefix: String(state ?? "").slice(0, 6),
        returnTo_session: returnToAtEnter,
      });

      // global lock
      const gotGlobal = acquireGlobalLock();
      authLog(runId, "callback:global_lock", { acquired: gotGlobal });
      if (!gotGlobal) return;

      // session lock（StrictMode二重・HMR二重の保険）
      const sessionLock = getSessionItem(EXCHANGE_LOCK_KEY);
      authLog(runId, "callback:session_lock_before", { sessionLock });
      if (sessionLock === "1") {
        releaseGlobalLock();
        return;
      }
      setSessionItem(EXCHANGE_LOCK_KEY, "1");

      if (exchangeInFlight.current) {
        authLog(runId, "callback:exchange_in_flight_skip");
        releaseGlobalLock();
        return;
      }
      exchangeInFlight.current = true;

      const go = (to: string) => {
        authLog(runId, "nav:replace", { to });
        navOnce(router, to);
        clearNavLockSoon();
      };

      try {
        if (!auth0Domain || !clientId || !audience) {
          authErr(runId, "env:missing", {
            auth0DomainPresent: !!auth0Domain,
            clientIdPresent: !!clientId,
            audiencePresent: !!audience,
          });
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          // RETURN_TO は消す（ここは復旧不能）
          removeSessionItem(RETURN_TO_KEY);
          go("/login?oidc_error=env_missing");
          return;
        }

        const expectedState = getSessionItem(OIDC_STATE_KEY);
        authLog(runId, "callback:state_check", {
          expectedPrefix: String(expectedState ?? "").slice(0, 6),
          actualPrefix: String(state ?? "").slice(0, 6),
          matches: !!expectedState && state === expectedState,
        });

        if (!expectedState || state !== expectedState) {
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          go("/login?oidc_error=state_mismatch");
          return;
        }

        const verifier = getSessionItem(PKCE_VERIFIER_KEY);
        authLog(runId, "callback:verifier_present", {
          hasVerifier: !!verifier,
        });

        if (!verifier) {
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          go("/login?oidc_error=missing_verifier");
          return;
        }

        // token exchange
        authLog(runId, "token_exchange:request", {
          tokenEndpoint: endpoints.token,
          redirectUri,
        });

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

        authLog(runId, "token_exchange:response", {
          ok: res.ok,
          status: res.status,
        });

        if (!res.ok) {
          const text = await res.text().catch(() => "");
          authErr(runId, "token_exchange:failed", {
            status: res.status,
            detail: text.slice(0, 200),
          });
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          go(`/login?oidc_error=token_exchange_failed&status=${res.status}`);
          return;
        }

        const json = (await res.json().catch(() => ({}))) as any;
        const accessToken =
          typeof json?.access_token === "string" ? json.access_token : "";
        const refreshToken =
          typeof json?.refresh_token === "string" ? json.refresh_token : "";

        authLog(runId, "token_exchange:tokens", {
          hasAccessToken: !!accessToken,
          hasRefreshToken: !!refreshToken,
          accessTokenMask: maskToken(accessToken),
        });

        if (!accessToken) {
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          go("/login?oidc_error=missing_access_token");
          return;
        }

        // ✅ token 保存（ここから先、me失敗で即 /login へ逃がさない）
        TokenStorage.save({ accessToken, refreshToken });

        // ✅ callback専用：me リトライ（401/403も短く待つ）
        const meResult = await fetchMeWithRetryAfterExchange(runId, 3);

        // ✅ RETURN_TO は “必ず” ここから取る（hold URL の唯一の根拠）
        const returnToRaw = getSessionItem(RETURN_TO_KEY);
        const returnTo = safeReturnTo(returnToRaw);

        if (!meResult.ok) {
          authErr(runId, "me:failed_after_exchange", meResult);

          // ここが最重要：hold があるなら、login再発火で returnTo を壊さない
          if (returnTo && isHoldReturnTo(returnTo)) {
            authLog(runId, "me:fail_redirect_to_hold", { returnTo });
            clearOidcSessionStateAfterCallback();
            // RETURN_TO_KEY は “踏んだ後”に消す
            go(returnTo);
            clearReturnToSoon();
            return;
          }

          // hold が無いなら初めて /login に落とす
          if (meResult.kind === "unauthorized") {
            TokenStorage.clear();
            clearOidcSessionStateAfterCallback();
            removeSessionItem(RETURN_TO_KEY);
            go("/login?reason=me_unauthorized");
            return;
          }

          clearOidcSessionStateAfterCallback();
          // tokenは保持（transient扱い）
          go("/login?reason=me_transient");
          return;
        }

        const me = meResult.me;

        const completed = normalizeBool((me as any)?.profile_completed);
        authLog(runId, "redirect:pre", {
          returnToRaw,
          returnTo,
          completed,
          isHold: isHoldReturnTo(returnTo),
        });

        // ✅ 1) hold は最優先で必ず踏む（ここで 10 秒 UI が保証される）
        if (returnTo && isHoldReturnTo(returnTo)) {
          clearOidcSessionStateAfterCallback();
          go(returnTo);
          clearReturnToSoon();
          clearJustLoggedInSoon();
          return;
        }

        // ✅ 2) 通常 returnTo があるならそれを踏む（/ 以外）
        if (returnTo && returnTo !== "/") {
          clearOidcSessionStateAfterCallback();
          go(returnTo);
          clearReturnToSoon();
          clearJustLoggedInSoon();
          return;
        }

        // ✅ 3) / の場合のみ既存の owner redirect
        const shopCode = pickOwnerShopCode(me as any);
        if (shopCode) {
          authLog(runId, "redirect:owner_dashboard", { shopCode });
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          setSessionItem(JUST_LOGGED_IN_KEY, "1");
          setSessionItem(OWNER_REDIRECT_KEY, shopCode);
          window.location.assign(`/shops/${shopCode}/dashboard`);
          return;
        }

        authLog(runId, "redirect:home", {});
        clearOidcSessionStateAfterCallback();
        removeSessionItem(RETURN_TO_KEY);
        go("/");
        clearJustLoggedInSoon();
      } finally {
        exchangeInFlight.current = false;
        // EXCHANGE_LOCK_KEY は残す（次回login()が必ず掃除する）
        releaseGlobalLock();
      }
    },
    [
      auth0Domain,
      clientId,
      audience,
      endpoints.token,
      redirectUri,
      router,
      fetchMeWithRetryAfterExchange,
    ],
  );

  useEffect(() => {
    const runId = makeRunId("effect");
    runIdRef.current = runId;

    let aborted = false;

    (async () => {
      try {
        const code = searchParams.get("code");
        const state = searchParams.get("state");
        const error = searchParams.get("error");
        const errorDescription = searchParams.get("error_description");

        authLog(runId, "effect:enter", {
          href: typeof window !== "undefined" ? window.location.href : "",
          hasCode: !!code,
          hasState: !!state,
          error: error ?? null,
          tokenPresent: !!TokenStorage.load().accessToken,
          returnTo: getSessionItem(RETURN_TO_KEY),
          exchangeLock: getSessionItem(EXCHANGE_LOCK_KEY),
        });

        if (error) {
          authErr(runId, "callback:error_from_auth0", {
            error,
            errorDescription,
          });
          TokenStorage.clear();
          clearOidcSessionStateAfterCallback();
          removeSessionItem(RETURN_TO_KEY);
          router.replace(
            `/login?oidc_error=${encodeURIComponent(error)}${
              errorDescription
                ? `&oidc_error_description=${encodeURIComponent(errorDescription)}`
                : ""
            }`,
          );
          return;
        }

        if (code) {
          await handleCallback({ code, state });
          return;
        }

        // normal init
        const { accessToken } = TokenStorage.load();
        if (accessToken) {
          authLog(runId, "init:token_present_fetch_me");
          await fetchMeNormalInit(runId);
        } else {
          authLog(runId, "init:no_token");
        }
      } catch (e: any) {
        authErr(runId, "effect:unhandled", {
          message: String(e?.message ?? e),
        });
      } finally {
        if (aborted) return;
        setIsLoading(false);
        setAuthReady(true);
        authLog(runId, "effect:done", { isLoading: false, authReady: true });
      }
    })();

    return () => {
      aborted = true;
    };
  }, [searchParams, router, handleCallback, fetchMeNormalInit]);

  const login = useCallback(
    async (payload: LoginPayload) => {
      const runId = makeRunId("login");
      runIdRef.current = runId;

      const returnToFromPayload =
        payload.type === "oidc" ? payload.returnTo : undefined;

      if (!auth0Domain || !clientId) {
        authErr(runId, "login:env_missing", {
          auth0DomainPresent: !!auth0Domain,
          clientIdPresent: !!clientId,
        });
        throw new Error(
          "Auth0 env missing: NEXT_PUBLIC_AUTH0_DOMAIN / NEXT_PUBLIC_AUTH0_CLIENT_ID",
        );
      }
      if (!audience) {
        authErr(runId, "login:env_missing_audience");
        throw new Error("Auth0 env missing: NEXT_PUBLIC_AUTH0_AUDIENCE");
      }

      const verifier = randomString(64);
      const challenge = await sha256Base64Url(verifier);

      try {
        // ✅ 毎回クリーンに（callbackのロック類もここで必ず落とす）
        removeSessionItem(EXCHANGE_LOCK_KEY);
        releaseGlobalLock();

        removeSessionItem(NAV_LOCK_KEY);
        removeSessionItem(OWNER_REDIRECT_KEY);
        removeSessionItem(JUST_LOGGED_IN_KEY);

        removeSessionItem(RETURN_TO_KEY);
        removeSessionItem(OIDC_STATE_KEY);
        removeSessionItem(PKCE_VERIFIER_KEY);

        setSessionItem(PKCE_VERIFIER_KEY, verifier);

        const s = randomString(32);
        setSessionItem(OIDC_STATE_KEY, s);

        const injected = safeReturnTo(returnToFromPayload ?? null);

        const currentPath =
          typeof window !== "undefined"
            ? `${window.location.pathname}${window.location.search}`
            : "/";
        const fallback = currentPath.startsWith("/login") ? "/" : currentPath;

        let returnTo = injected !== "/" ? injected : fallback;
        if (returnTo === "/mypage/profile") returnTo = "/";

        // ✅ returnTo は単一キーに保存（hold URL もここに入る）
        setSessionItem(RETURN_TO_KEY, returnTo);

        authLog(runId, "login:start", {
          injected: returnToFromPayload ?? null,
          fallback,
          finalReturnTo: returnTo,
          redirectUri,
          scopes,
          audiencePresent: !!audience,
        });

        const params = new URLSearchParams();
        params.set("response_type", "code");
        params.set("client_id", clientId);
        params.set("redirect_uri", redirectUri);
        params.set("scope", scopes);
        params.set("audience", audience);
        params.set("code_challenge", challenge);
        params.set("code_challenge_method", "S256");
        params.set("state", s);

        authLog(runId, "login:redirect_to_auth0", {
          authorize: endpoints.authorize,
          statePrefix: s.slice(0, 6),
          hasVerifier: true,
          hasChallenge: true,
        });

        window.location.assign(`${endpoints.authorize}?${params.toString()}`);
      } catch (e: any) {
        authErr(runId, "login:failed", { message: String(e?.message ?? e) });
        TokenStorage.clear();
        clearOidcSessionStateAfterCallback();
        removeSessionItem(RETURN_TO_KEY);
        throw new Error("auth0_login_start_failed");
      }
    },
    [auth0Domain, clientId, audience, redirectUri, scopes, endpoints.authorize],
  );

  const logout = useCallback(async () => {
    const runId = makeRunId("logout");
    authLog(runId, "logout:start");

    TokenStorage.clear();
    clearOidcSessionStateAfterCallback();
    removeSessionItem(RETURN_TO_KEY);
    setUser(null);

    removeSessionItem(NAV_LOCK_KEY);
    removeSessionItem(OWNER_REDIRECT_KEY);
    removeSessionItem(JUST_LOGGED_IN_KEY);

    if (!auth0Domain || !clientId) {
      authLog(runId, "logout:local_only");
      router.replace("/login");
      return;
    }

    const params = new URLSearchParams();
    params.set("client_id", clientId);
    params.set("returnTo", postLogoutRedirectUri);

    authLog(runId, "logout:redirect", { to: endpoints.logout });
    window.location.assign(`${endpoints.logout}?${params.toString()}`);
  }, [auth0Domain, clientId, endpoints.logout, postLogoutRedirectUri, router]);

  const value: AuthContext = useMemo(
    () => ({
      isLoading,
      authReady,
      isAuthenticated: !!user,
      user,
      apiClient,
      login,
      logout,
      refresh,
    }),
    [isLoading, authReady, user, apiClient, login, logout, refresh],
  );

  return (
    <AuthContextCoreProvider value={value}>{children}</AuthContextCoreProvider>
  );
}
