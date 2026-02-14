"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";
import styles from "./W-AuthCallbackPage.module.css";

// --------------------
// Constants (shared keys)
// --------------------
const FLOW_KEY = "occore_email_flow_v2";
const NONCE_KEY = "occore_email_flow_nonce_v1";
const RETURN_TO_KEY = "auth0_return_to_v1"; // ✅ AuthProvider と必ず一致

type FlowPhase = "first" | "second_pending" | "second_done";
type Screen = "login" | "verify" | "verify_done";

const STEP1_MS = 10_000;
const STEP2_MS = 10_000;
const TICK_MS = 100;

const PHASE_QUERY_KEY = "p";
const NONCE_QUERY_KEY = "nonce";

type FlowState = { phase: FlowPhase; ts: number };

// --------------------
// Debug helper
// --------------------
function clearReturnToFromSession() {
  try {
    sessionStorage.removeItem(RETURN_TO_KEY);
  } catch {}
}

function d(...args: any[]) {
  console.log("[AuthCallback]", ...args);
}

function clamp(n: number, a = 0, b = 1) {
  return Math.max(a, Math.min(b, n));
}

function readFlowState(): FlowState | null {
  try {
    const raw = localStorage.getItem(FLOW_KEY);
    if (!raw) return null;
    const v = JSON.parse(raw);
    const p = v?.phase as FlowPhase | undefined;
    const ts = typeof v?.ts === "number" ? v.ts : 0;
    if (p === "first" || p === "second_pending" || p === "second_done")
      return { phase: p, ts };
    return null;
  } catch {
    return null;
  }
}

function writeFlow(phase: FlowPhase) {
  try {
    localStorage.setItem(FLOW_KEY, JSON.stringify({ phase, ts: Date.now() }));
  } catch {}
}

function clearFlow() {
  try {
    localStorage.removeItem(FLOW_KEY);
  } catch {}
}

function genNonce(): string {
  try {
    const c: any = globalThis.crypto;
    if (c?.randomUUID) return c.randomUUID();
  } catch {}
  return `${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function getNonceFromSession(): string | null {
  try {
    return sessionStorage.getItem(NONCE_KEY);
  } catch {
    return null;
  }
}

function setNonceToSession(nonce: string) {
  try {
    sessionStorage.setItem(NONCE_KEY, nonce);
  } catch {}
}

function clearNonceFromSession() {
  try {
    sessionStorage.removeItem(NONCE_KEY);
  } catch {}
}

function setReturnToToSession(returnTo: string) {
  try {
    sessionStorage.setItem(RETURN_TO_KEY, returnTo);
  } catch {}
}

function isValidPhase(v: string | null): v is FlowPhase {
  return v === "first" || v === "second_pending" || v === "second_done";
}

/**
 * ✅ “必ず 10秒表示” させる hold URL を作る（p が主語）
 */
function buildHoldUrl(args: {
  phase: FlowPhase;
  finalReturnTo: string;
  nonce?: string | null;
}): string {
  const { phase, finalReturnTo, nonce } = args;
  const sp = new URLSearchParams();
  sp.set(PHASE_QUERY_KEY, phase);
  sp.set("returnTo", finalReturnTo);
  if (nonce) sp.set(NONCE_QUERY_KEY, nonce);

  // 互換で screen も付ける（ただし主語は p）
  sp.set("screen", phase === "second_done" ? "verify_done" : "verify");
  return `/auth/callback?${sp.toString()}`;
}

const normalizeInternalPath = (raw?: string | null) => {
  if (!raw) return null;
  if (!raw.startsWith("/")) return null; // 外部URL拒否（安全）
  try {
    const u = new URL(raw, "http://local");
    return `${u.pathname}${u.search}${u.hash}`;
  } catch {
    return null;
  }
};

const resolvePostVerifyReturnTo = (me: any, raw: string | null) => {
  const normalized = normalizeInternalPath(raw);

  // 今回の事故（存在しないページ）を確実に回避
  if (!normalized || normalized === "/email/verify-second") {
    return me?.profile_completed_norm ? "/" : "/mypage/profile";
  }
  return normalized;
};

export default function AuthCallbackPage() {
  const sp = useSearchParams();
  const router = useRouter();
  const { login, refresh } = useAuth();

  const returnToRaw = normalizeInternalPath(sp.get("returnTo")) ?? "/";

  // Auth0 callback params（ここに来ても “このページでは何もしない”）
  const code = sp.get("code");
  const error = sp.get("error");

  // ✅ hold params
  const phaseFromQuery = sp.get(PHASE_QUERY_KEY);
  const phaseFromQuerySafe: FlowPhase | null = isValidPhase(phaseFromQuery)
    ? phaseFromQuery
    : null;
  const nonceFromQuery = sp.get(NONCE_QUERY_KEY);

  // ✅ 4枚目（code callbackでp無し）だけを“ほぼ無表示”にする判定
  const isSilentTransit = !!code && !phaseFromQuerySafe;

  // --------------------
  // ✅ Debug: ローカル強制シーン表示（登録/ログイン不要）
  // ?debugScene=s3&debugHold=1[&debugPhase=second_done]
  // - debugScene: s1 | s1r | s3 | sx
  // - debugHold=1: ストーリー(10s)を走らせる（自動遷移は無効化）
  // --------------------
  const debugSceneParam = sp.get("debugScene");
  const debugHoldParam = sp.get("debugHold");
  const debugPhaseParam = sp.get("debugPhase");
  const debugPhaseSafe: FlowPhase | null = isValidPhase(debugPhaseParam)
    ? debugPhaseParam
    : null;

  const debugSceneKind = useMemo<"s1" | "s1r" | "s3" | "sx" | null>(() => {
    if (debugSceneParam === "s1") return "s1";
    if (debugSceneParam === "s1r") return "s1r";
    if (debugSceneParam === "s3") return "s3";
    if (debugSceneParam === "sx") return "sx";
    return null;
  }, [debugSceneParam]);

  const isDebugScene = !!debugSceneKind;
  const isDebugHold = debugHoldParam === "1";

  const debugEffectivePhase: FlowPhase | null = useMemo(() => {
    if (!isDebugScene) return null;
    if (debugSceneKind === "sx") return null; // login相当
    if (!isDebugHold) return null; // holdしない=ストーリー走らない
    return debugPhaseSafe ?? "first";
  }, [isDebugScene, debugSceneKind, isDebugHold, debugPhaseSafe]);

  const [hydrated, setHydrated] = useState(false);
  const [phase, setPhase] = useState<FlowPhase | null>(null);

  // --------------------
  // hydrate
  // --------------------
  useEffect(() => {
    setHydrated(true);
    const st = readFlowState();
    setPhase(st?.phase ?? null);

    d("hydrate", {
      href: typeof window !== "undefined" ? window.location.href : "",
      flowLocal: st,
      phaseFromQuerySafe,
      hasCode: !!code,
      hasError: !!error,
      returnToRaw,
      nonceFromQuery,
      isSilentTransit,

      // debug
      isDebugScene,
      debugSceneKind,
      isDebugHold,
      debugPhaseSafe,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --------------------
  // ✅ effectivePhase: “pが無い /auth/callback” を first として救済
  // --------------------
  const effectivePhase: FlowPhase | null = useMemo(() => {
    // ✅ Debugが指定されているときは SoT を debug に固定
    if (isDebugScene) return debugEffectivePhase;

    // ✅ URL(p) があればそれが絶対SoT
    if (phaseFromQuerySafe) return phaseFromQuerySafe;

    // ✅ code がある場合は OIDC callback → IdaasProvider が hold に飛ばすのが正
    if (code) return null;

    // ✅ error は login へ戻すUIにしたいなら null のままでOK
    if (error) return null;

    // ✅ p無しで /auth/callback に来た = 迷子/verify link戻り
    // いきなり first 固定にせず、localStorage(FLOW_KEY)で最後に確定したphaseを優先
    // phase state は hydrate の readFlowState() 由来
    return phase ?? "first";
  }, [
    isDebugScene,
    debugEffectivePhase,
    phaseFromQuerySafe,
    code,
    error,
    phase, // ✅ 追加
  ]);

  // --------------------
  // ✅ phase の SoT: p があれば採用。p無しだけど effectivePhase=first なら holdへ正規化
  // --------------------
  useEffect(() => {
    if (!hydrated) return;

    // ✅ Debug中は URL/flow をいじらない（眺めるだけ）
    if (isDebugScene) {
      d("debug:skip_phase_sync", { debugSceneKind, isDebugHold });
      return;
    }

    // ✅ p があるならそれをSoTにする
    if (phaseFromQuerySafe) {
      d("phase:adopt_from_query", {
        phaseFromQuerySafe,
        nonceFromQuery,
      });

      writeFlow(phaseFromQuerySafe);
      setPhase(phaseFromQuerySafe);

      // second_pending の nonce を session に保持（次の second_done 判定の鍵）
      if (phaseFromQuerySafe === "second_pending" && nonceFromQuery) {
        setNonceToSession(nonceFromQuery);
        d("nonce:stored_to_session", { nonceFromQuery });
      }
      return;
    }

    // ✅ p無し + code無し + error無し：迷子救済 → hold(p=...)に正規化
    // effectivePhase は localStorage(flow)優先になっているので巻き戻りしない
    if (!code && !error && effectivePhase) {
      let nonce: string | null = null;

      // second_pending の時は nonce が必要になりがち（別タブで sessionStorage 空対策）
      if (effectivePhase === "second_pending") {
        nonce = getNonceFromSession();
        if (!nonce) {
          nonce = genNonce();
          setNonceToSession(nonce);
          d("nonce:generated_for_repair", { nonce });
        }
      }

      const hold = buildHoldUrl({
        phase: effectivePhase,
        finalReturnTo: returnToRaw,
        nonce,
      });

      d("phase:repair_no_p_no_code", {
        action: "router.replace(hold)",
        to: hold,
        effectivePhase,
        returnToRaw,
        nonce,
      });

      router.replace(hold);
      return;
    }

    // ✅ p無し + codeあり は何もしない（IdaasProvider が hold に飛ばす）
    d("phase:no_p", {
      hasCode: !!code,
      hasError: !!error,
      effectivePhase,
      note: "waiting for IdaasProvider to redirect to hold",
    });
  }, [
    hydrated,
    isDebugScene,
    debugSceneKind,
    isDebugHold,
    phaseFromQuerySafe,
    nonceFromQuery,
    code,
    error,
    effectivePhase,
    returnToRaw,
    router,
  ]);

  // --------------------
  // screen/duration: effectivePhase 基準（Debug時は override）
  // --------------------
  const screen: Screen = useMemo(() => {
    // ✅ Debug
    if (isDebugScene) {
      if (debugSceneKind === "sx") return "login";
      if (debugEffectivePhase === "second_done") return "verify_done";
      return "verify";
    }

    if (effectivePhase === "first") return "verify";
    if (effectivePhase === "second_pending") return "verify";
    if (effectivePhase === "second_done") return "verify_done";
    return "login";
  }, [isDebugScene, debugSceneKind, debugEffectivePhase, effectivePhase]);

  const isVerify = screen === "verify" || screen === "verify_done";

  const durationMs = useMemo(() => {
    // ✅ Debug: debugHold=1 の時だけストーリーを走らせる
    if (isDebugScene) {
      if (!isDebugHold) return 0;
      if (debugEffectivePhase === "second_done") return STEP2_MS;
      return STEP1_MS;
    }

    if (effectivePhase === "second_done") return STEP2_MS;
    if (effectivePhase === "first" || effectivePhase === "second_pending")
      return STEP1_MS;
    return 0;
  }, [isDebugScene, isDebugHold, debugEffectivePhase, effectivePhase]);

  // --------------------
  // ✅ Scene selection (S1 / S1R / S3 / SX)
  // --------------------
  const sceneKind = useMemo<"s1" | "s1r" | "s3" | "sx">(() => {
    // ✅ Debug
    if (isDebugScene && debugSceneKind) return debugSceneKind;

    if (!isVerify) return "sx";
    if (effectivePhase === "first") return "s1"; // 1つ目：現状
    if (effectivePhase === "second_pending") return "s1r"; // 2つ目：逆走
    if (effectivePhase === "second_done") return "s3"; // 3つ目：PC+Phone+Lang
    return "sx";
  }, [isDebugScene, debugSceneKind, isVerify, effectivePhase]);

  // --------------------
  // ✅ UI texts (phaseごとにテキストのみ変える)
  // --------------------
  const stepLabel = useMemo(() => {
    if (effectivePhase === "first") return "STEP 1/2";
    if (effectivePhase === "second_pending") return "STEP 1.5/2";
    if (effectivePhase === "second_done") return "STEP 2/2";
    return "AUTH";
  }, [effectivePhase]);

  const title = useMemo(() => {
    if (effectivePhase === "first") return "メール認証 1/2";
    if (effectivePhase === "second_pending") return "メール認証 1.5/2";
    if (effectivePhase === "second_done") return "メール認証 2/2(待ち時間,言語をhoverできます。)";
    return "Signing in…";
  }, [effectivePhase]);

  const baseMessage = useMemo(() => {
    if (effectivePhase === "first")
      return "メール認証 1/2 が完了しました。次は 1.5/2 の登録へ進みます。";
    if (effectivePhase === "second_pending")
      return "1.5/2 の登録が完了しました。届いたメールの Verify Link を押して完了してください。";
    if (effectivePhase === "second_done")
      return "メール認証 2/2 が完了しました。10秒後にログイン処理を開始します。";

    if (code)
      return "認証情報を受け取りました。処理中…（この後ホールド画面に遷移します）";
    if (error) return "SSOでエラーが発生しました。ログイン画面に戻ります…";
    return "SSO認証を開始しています。";
  }, [effectivePhase, code, error]);

  // --------------------
  // Refs/timers
  // --------------------
  const rootRef = useRef<HTMLDivElement | null>(null);
  const storyRef = useRef<HTMLDivElement | null>(null);
  const readyRef = useRef(false);
  const onceRef = useRef(false);
  const rafRef = useRef<number | null>(null);
  const tickTimerRef = useRef<number | null>(null);
  const afterReadyTimerRef = useRef<number | null>(null);
  const startedAtRef = useRef<number>(0);

  const qs = useCallback(
    <T extends Element>(sel: string) =>
      rootRef.current?.querySelector<T>(sel) ?? null,
    [],
  );

  const setCssVar = useCallback((name: string, value: string) => {
    const el = (storyRef.current ?? rootRef.current) as HTMLElement | null;
    if (!el) return;
    el.style.setProperty(name, value);
  }, []);

  const setDomText = useCallback(
    (sel: string, text: string) => {
      const t = qs<HTMLElement>(sel);
      if (t) t.textContent = text;
    },
    [qs],
  );

  const setBtnDisabled = useCallback(
    (disabled: boolean) => {
      const btn = qs<HTMLButtonElement>(`[data-el="continueBtn"]`);
      if (!btn) return;
      btn.disabled = disabled;
      btn.setAttribute("aria-disabled", disabled ? "true" : "false");
      btn.classList.toggle(styles.btnDisabled, disabled);
    },
    [qs],
  );

  const clearTimers = useCallback(() => {
    if (rafRef.current) cancelAnimationFrame(rafRef.current);
    rafRef.current = null;
    if (tickTimerRef.current) window.clearInterval(tickTimerRef.current);
    tickTimerRef.current = null;
    if (afterReadyTimerRef.current)
      window.clearTimeout(afterReadyTimerRef.current);
    afterReadyTimerRef.current = null;
  }, []);

  // --------------------
  // Actions (機能そのまま)
  // --------------------

  /**
   * ✅ 1/2 完走後 → 2/2 signup を開始
   * callback 後は必ず second_pending の hold に戻す（=10秒表示）
   */
  const goToAuth0SignupForSecond = useCallback(async () => {
    if (isDebugScene) {
      d("debug:block_action:goToAuth0SignupForSecond");
      return;
    }
    if (onceRef.current) return;
    onceRef.current = true;

    const nonce = genNonce();
    setNonceToSession(nonce);

    const holdPending = buildHoldUrl({
      phase: "second_pending",
      finalReturnTo: returnToRaw,
      nonce,
    });

    // ✅ ここは「毎回上書き」なので古いのを消してから入れるのが安全
    clearReturnToFromSession();
    setReturnToToSession(holdPending);

    writeFlow("second_pending");
    setPhase("second_pending");

    d("action:goToAuth0SignupForSecond", { nonce, holdPending, returnToRaw });

    try {
      await login({ type: "oidc", returnTo: holdPending });
    } catch (e) {
      console.error(e);

      // ✅ 失敗時は詰まりを消す（再試行可能に）
      onceRef.current = false;
      clearFlow();
      clearNonceFromSession();
      clearReturnToFromSession();
      setPhase(null);

      router.replace(`/login?oidc_error=1`);
    }
  }, [isDebugScene, login, router, returnToRaw]);

  /**
   * ✅ second_pending になったら、次の callback 後の着地点を “second_done hold” に仕込む
   */
  useEffect(() => {
    if (!hydrated) return;

    // ✅ Debug中は外部遷移準備もしない
    if (isDebugScene) return;

    if (effectivePhase !== "second_pending") return;

    const nonce = getNonceFromSession();
    if (!nonce) {
      d("arm:second_done_hold:missing_nonce", {
        note: "waiting for nonce set by first flow",
      });
      return;
    }

    const holdDone = buildHoldUrl({
      phase: "second_done",
      finalReturnTo: returnToRaw,
      nonce,
    });

    // ✅ 次の code callback 完了後、AuthProvider がここへ replace する
    setReturnToToSession(holdDone);

    d("arm:second_done_hold:ok", { holdDone, nonce, returnToRaw });
  }, [hydrated, isDebugScene, effectivePhase, returnToRaw]);

  /**
   * ✅ 2/2 完走後 → 最終ログイン（callback後は final returnTo へ）
   */
  const finalizeAfterSecondDone = useCallback(async () => {
    if (isDebugScene) {
      d("debug:block_action:finalizeAfterSecondDone");
      return;
    }
    if (onceRef.current) return;
    onceRef.current = true;

    // ✅ flow掃除（ここを強化）
    clearFlow();
    clearNonceFromSession();
    clearReturnToFromSession();
    setPhase(null);

    d("action:finalizeAfterSecondDone", { returnToRaw });

    let me: any = null;
    try {
      me = await refresh?.();
    } catch {}

    const next = resolvePostVerifyReturnTo(me, returnToRaw);
    router.replace(next || "/");
  }, [isDebugScene, refresh, router, returnToRaw]);

  // --------------------
  // story runner
  // --------------------
  const startStory = useCallback(() => {
    if (!isVerify) return;
    if (!durationMs) return;

    const dur = durationMs;
    const start = performance.now();
    startedAtRef.current = start;

    d("story:start", {
      effectivePhase,
      screen,
      durationMs: dur,
      returnToRaw,
      href: typeof window !== "undefined" ? window.location.href : "",
      isDebugScene,
      debugSceneKind,
      isDebugHold,
    });

    setCssVar("--p", "0");
    setDomText(`[data-el="secondsLeft"]`, String(Math.ceil(dur / 1000)));
    setDomText(`[data-el="hintText"]`, "続けるまで");
    setDomText(`[data-el="statusText"]`, "PREPARING");
    setDomText(`[data-el="messageText"]`, baseMessage);

    setBtnDisabled(true);
    readyRef.current = false;

    tickTimerRef.current = window.setInterval(() => {
      const now = performance.now();
      const elapsed = now - startedAtRef.current;
      const left = Math.max(0, Math.ceil((dur - elapsed) / 1000));
      setDomText(`[data-el="secondsLeft"]`, String(left));
    }, TICK_MS);

    const tick = (now: number) => {
      const elapsed = now - start;
      const p = clamp(elapsed / dur, 0, 1);
      setCssVar("--p", String(p));

      if (p >= 1) {
        if (tickTimerRef.current) window.clearInterval(tickTimerRef.current);
        tickTimerRef.current = null;

        readyRef.current = true;
        setBtnDisabled(false);
        setDomText(`[data-el="statusText"]`, "READY");
        setDomText(`[data-el="hintText"]`, "準備完了");

        d("story:ready", { effectivePhase, isDebugScene });

        // ✅ Debug中は “自動遷移” を完全に無効化（眺めるだけ）
        if (isDebugScene) return;

        afterReadyTimerRef.current = window.setTimeout(() => {
          if (effectivePhase === "first") void goToAuth0SignupForSecond();
          if (effectivePhase === "second_done") void finalizeAfterSecondDone();
          // second_pending は待機
        }, 0);

        return;
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
  }, [
    isVerify,
    durationMs,
    effectivePhase,
    screen,
    returnToRaw,
    setCssVar,
    setDomText,
    setBtnDisabled,
    baseMessage,
    goToAuth0SignupForSecond,
    finalizeAfterSecondDone,
    isDebugScene,
    debugSceneKind,
    isDebugHold,
  ]);

  // --------------------
  // init
  // --------------------
  const isHold = !!effectivePhase;

  useEffect(() => {
    clearTimers();
    onceRef.current = false;

    d("screen:init", {
      href: typeof window !== "undefined" ? window.location.href : "",
      hydrated,
      phaseState: phase,
      effectivePhase,
      screen,
      isVerify,
      durationMs,
      hasP: !!phaseFromQuerySafe,
      hasCode: !!code,
      hasError: !!error,
      isHold,
      returnToRaw,
      flowLocal: readFlowState(),
      nonce_session: getNonceFromSession(),
      nonce_query: nonceFromQuery,
      sceneKind,
      isSilentTransit,

      // debug
      isDebugScene,
      debugSceneKind,
      isDebugHold,
      debugPhaseSafe,
    });

    if (isVerify && isHold) {
      startStory();
      return () => clearTimers();
    }

    return () => clearTimers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    hydrated,
    phase,
    effectivePhase,
    screen,
    isVerify,
    durationMs,
    phaseFromQuerySafe,
    code,
    error,
    isHold,
    returnToRaw,
    nonceFromQuery,
    startStory,
    clearTimers,
    sceneKind,
    isSilentTransit,

    // debug
    isDebugScene,
    debugSceneKind,
    isDebugHold,
    debugPhaseSafe,
  ]);

  // --------------------
  // continue
  // --------------------
  const onContinue = useCallback(async () => {
    // ✅ Debug中はボタン押しても何もしない（眺めるだけ）
    if (isDebugScene) {
      d("debug:block_ui:onContinue");
      return;
    }

    if (!readyRef.current) return;

    d("ui:continue_clicked", { effectivePhase, returnToRaw });

    if (effectivePhase === "first") return void goToAuth0SignupForSecond();
    if (effectivePhase === "second_done") void finalizeAfterSecondDone();

    if (effectivePhase === "second_pending") {
      const nonce = getNonceFromSession() || genNonce();
      setNonceToSession(nonce);

      const holdDone = buildHoldUrl({
        phase: "second_done",
        finalReturnTo: returnToRaw,
        nonce,
      });

      clearReturnToFromSession();
      setReturnToToSession(holdDone);

      d("ui:continue_second_pending_to_done", { nonce, holdDone });

      try {
        await login({ type: "oidc", returnTo: holdDone });
      } catch (e) {
        console.error(e);

        onceRef.current = false;
        clearFlow();
        clearNonceFromSession();
        clearReturnToFromSession();
        setPhase(null);

        router.replace(`/login?oidc_error=1`);
      }
      return;
    }
  }, [
    isDebugScene,
    effectivePhase,
    returnToRaw,
    goToAuth0SignupForSecond,
    finalizeAfterSecondDone,
    login,
    router,
  ]);

  // --------------------
  // ✅ hydration guard
  // - 4枚目（codeあり & p無し）の瞬間は“透明”だけ返す
  // --------------------
  if (!hydrated) {
    // ✅ Debug中は常に描画する（透明にしない）
    if (!isDebugScene && isSilentTransit) {
      return (
        <div className={styles.silentTransit} role="status" aria-live="polite">
          <span className={styles.srOnly}>Signing in…</span>
        </div>
      );
    }

    return (
      <div className={styles.wrap}>
        <div className={styles.bgGlow} aria-hidden="true" />
        <div className={styles.grid} aria-hidden="true" />
        <div className={styles.card} role="status" aria-live="polite">
          <div className={styles.top}>
            <div className={styles.badge}>AUTH</div>
            <div className={styles.rightTop}>
              <div className={styles.tiny}>WORKING</div>
              <div className={styles.spinner} aria-hidden="true" />
            </div>
          </div>
          <h1 className={styles.title}>Loading…</h1>
          <p className={styles.message}>状態を確認しています…</p>
          <div className={styles.progressWrap}>
            <div className={styles.progressLoop}>
              <div className={styles.barLoop} />
            </div>
          </div>
        </div>
      </div>
    );
  }

  // --------------------
  // ✅ hydrated後も 4枚目（codeあり & p無し）は“透明”だけ返す
  // --------------------
  if (!isDebugScene && isSilentTransit) {
    return (
      <div className={styles.silentTransit} role="status" aria-live="polite">
        <span className={styles.srOnly}>Signing in…</span>
      </div>
    );
  }

  // --------------------
  // ✅ render
  // - verify系：S1 / S1R / S3 を effectivePhase で切替（sceneKind）
  // - login：SX のまま
  // --------------------
  return (
    <div
      ref={rootRef}
      className={styles.wrap}
      data-screen={screen}
      data-ready="0"
      data-phase={isVerify ? "story" : "idle"}
      style={{ ["--dur" as any]: `${durationMs}ms` }}
    >
      <div className={styles.bgGlow} aria-hidden="true" />
      <div className={styles.grid} aria-hidden="true" />

      <div className={styles.card} role="status" aria-live="polite">
        <div className={styles.top}>
          <div className={styles.badge}>{stepLabel}</div>
          <div className={styles.rightTop}>
            <div className={styles.tiny} data-el="statusText">
              {isHold && isVerify ? "PREPARING" : "WORKING"}
            </div>
            <div className={styles.spinner} aria-hidden="true" />
          </div>
        </div>

        {/* ========= Story ========= */}
        <div
          ref={storyRef}
          className={styles.story}
          aria-hidden="true"
          style={{ ["--dur" as any]: `${durationMs}ms`, ["--p" as any]: 0 }}
        >
          <div className={styles.halo} />
          <div className={styles.halo2} />

          {/* ✅ verify/verify_done：S1 / S1R / S3 を切替 */}
          {isVerify ? (
            <div
              className={`${styles.scene} ${
                sceneKind === "s1"
                  ? styles.sceneS1
                  : sceneKind === "s1r"
                    ? styles.sceneS1R
                    : styles.sceneS3
              }`}
            >
              {/* ---------- S1（既存） ---------- */}
              {sceneKind === "s1" ? (
                <>
                  <div className={styles.trailOnce} />

                  <div className={styles.envelope}>
                    <div className={styles.envBody} />
                    <div className={styles.envFlap} />
                    <div className={styles.envLine} />
                    <div className={styles.envShadow} />
                  </div>

                  <div className={styles.planeOnce}>
                    <div className={styles.planeBody} />
                    <div className={styles.planeWing} />
                    <div className={styles.planeWing2} />
                  </div>

                  <div className={`${styles.sparkOnce} ${styles.sA}`} />
                  <div className={`${styles.sparkOnce} ${styles.sB}`} />
                  <div className={`${styles.sparkOnce} ${styles.sC}`} />
                  <div className={`${styles.sparkOnce} ${styles.sD}`} />
                  <div className={`${styles.sparkOnce} ${styles.sE}`} />
                </>
              ) : null}

              {/* ---------- S1R（追加：逆走） ---------- */}
              {sceneKind === "s1r" ? (
                <>
                  <div className={styles.trailOnceR} />

                  <div className={styles.envelope}>
                    <div className={styles.envBody} />
                    <div className={styles.envFlap} />
                    <div className={styles.envLine} />
                    <div className={styles.envShadow} />
                  </div>

                  <div className={styles.planeOnceR}>
                    <div className={styles.planeBody} />
                    <div className={styles.planeWing} />
                    <div className={styles.planeWing2} />
                  </div>

                  <div className={`${styles.sparkOnceR} ${styles.rA}`} />
                  <div className={`${styles.sparkOnceR} ${styles.rB}`} />
                  <div className={`${styles.sparkOnceR} ${styles.rC}`} />
                  <div className={`${styles.sparkOnceR} ${styles.rD}`} />
                  <div className={`${styles.sparkOnceR} ${styles.rE}`} />
                </>
              ) : null}

              {/* ---------- S3（追加：Devices + Lang + Emit） ---------- */}
              {sceneKind === "s3" ? (
                <>
                  <div className={styles.emitField} />
                  <div className={styles.emitFlash} />
                  <div className={styles.emitBeams} />

                  <div className={styles.emitRing} />
                  <div className={styles.emitRing2} />
                  <div className={styles.emitRing3} />

                  <div className={styles.emitParticles} />

                  <div className={styles.devicesCast}>
                    <div className={styles.devicesFloat}>
                      <div className={styles.devices}>
                        <div className={styles.laptop}>
                          <div className={styles.laptopTop} />
                          <div className={styles.laptopScreen} />
                          <div className={styles.laptopBase} />
                        </div>
                        <div className={styles.phone}>
                          <div className={styles.phoneBody} />
                          <div className={styles.phoneScreen} />
                          <div className={styles.phoneCam} />
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* =========================================================
                     ✅ Lang (FIXED): cast と float を wrapper 分離
                     - .langWrap: 位置だけ (left/top var)
                     - .langCast: 射出 transform (s3LangCast) + aura
                     - .langFloat: ふわふわ transform (langFloatA/B/C)
                     - .lang: 崩壊 (opacity/blur) etc
                     ========================================================= */}

                  <div className={`${styles.langWrap} ${styles.lJava}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>Java</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>Canada</span>
                            <span className={styles.langAltBottom}>
                              ジェームズ・ゴズリング
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lPython}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>Python</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>
                              Netherlands
                            </span>
                            <span className={styles.langAltBottom}>
                              グイド・ヴァン・ロッサム
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lPHP}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>PHP</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>
                              Greenland / Denmark
                            </span>
                            <span className={styles.langAltBottom}>
                              ラスマス・ラードフ
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lRuby}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>Ruby</span>

                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>JAPAN</span>
                            <span className={styles.langAltBottom}>
                              まつもとゆきひろ
                            </span>
                          </span>
                        </span>

                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lKotlin}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>Kotlin</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>
                              Russia / Czech
                            </span>
                            <span className={styles.langAltBottom}>
                              ジェットブレインズ社
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lGo}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>GO</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>
                              United States
                            </span>
                            <span className={styles.langAltBottom}>
                              ロバート・グリースマー
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>

                  <div className={`${styles.langWrap} ${styles.lElixir}`}>
                    <div className={styles.langCast}>
                      <div className={styles.langFloat}>
                        <span
                          className={`${styles.lang} ${styles.langHoverSwap}`}
                        >
                          <span className={styles.langFace}>Elixir</span>
                          <span className={styles.langAlt} aria-hidden="true">
                            <span className={styles.langAltTop}>Brazil</span>
                            <span className={styles.langAltBottom}>
                              ジョゼ・ヴァリム
                            </span>
                          </span>
                        </span>
                        <i className={styles.starBurst} aria-hidden="true" />
                      </div>
                    </div>
                  </div>
                </>
              ) : null}
            </div>
          ) : null}

          {/* login時（SXは既存のまま） */}
          {!isVerify ? (
            <div className={`${styles.scene} ${styles.sceneSX}`}>
              <div className={styles.waveOnce} />
              <div className={styles.waveOnce2} />
              <div className={`${styles.dotOnce} ${styles.d1}`} />
              <div className={`${styles.dotOnce} ${styles.d2}`} />
              <div className={`${styles.dotOnce} ${styles.d3}`} />
            </div>
          ) : null}
        </div>

        <h1 className={styles.title}>{title}</h1>

        <p className={styles.message} data-el="messageText">
          {baseMessage}
        </p>

        {/* ✅ instructions：phaseで文言だけ切替 */}
        {isVerify ? (
          <div className={styles.instructions}>
            <div className={styles.instTitle}>次の手順</div>

            {effectivePhase === "first" ? (
              <>
                <div className={styles.instBlock}>
                  <div className={styles.instLead}>
                    メール認証 1.5/2 登録の手順
                  </div>
                  <div className={styles.instText}>
                    ログインページの <b>”Continue”</b> の下の <b>”Sign up”</b>{" "}
                    を押して
                    <br />
                    メールとパスワードを再度入力して <b>メール認証 2/2</b>{" "}
                    の登録してください。
                  </div>
                </div>

                <div className={styles.instBlock}>
                  <div className={styles.instLead}>
                    その後メール認証 2/2 登録完了の手順
                  </div>
                  <ol className={styles.instList}>
                    <li>
                      <b>“OmniCommerce Core CLI_Native”</b> からメールが届きます
                    </li>
                    <li>
                      メールを開き <b>“Verify Link”</b> /{" "}
                      <b>“Verify Your Account”</b> をクリックしてください。
                    </li>
                  </ol>
                </div>
              </>
            ) : effectivePhase === "second_pending" ? (
              <div className={styles.instBlock}>
                <div className={styles.instLead}>メール認証 2/2 登録の手順</div>
                <div className={styles.instText}>
                  メールを開き <b>“Verify Link”</b> /{" "}
                  <b>“Verify Your Account”</b> をクリックしてください。
                  <br />
                  （この画面は待機します）
                </div>
              </div>
            ) : (
              // second_done
              <div className={styles.instBlock}>
                <div className={styles.instLead}>
                  メール認証完了 2/2 が全て完了しました。
                </div>
                <div className={styles.instText}>
                  10秒後にログイン処理を開始します。
                  <br />
                  自動で進まない場合は「続ける」を押してください。
                </div>
              </div>
            )}
          </div>
        ) : null}

        {/* ========= Verify flow UI ========= */}
        {isVerify ? (
          <>
            <div className={styles.progressWrap}>
              <div className={styles.progress}>
                <div className={styles.barOnce} />
                <div className={styles.glintOnce} />
              </div>

              <div className={styles.hint}>
                <span data-el="hintText">
                  {isHold ? "続けるまで" : "処理中"}
                </span>{" "}
                <b data-el="secondsLeft">{isHold ? 10 : 0}</b>{" "}
                {isHold ? "秒" : ""}
              </div>
            </div>

            <button
              data-el="continueBtn"
              onClick={onContinue}
              disabled={!isHold}
              className={`${styles.btn} ${!isHold ? styles.btnDisabled : ""}`}
            >
              <span className={styles.btnGlow} aria-hidden="true" />
              続ける
            </button>

            <div className={styles.sub}>
              {!isHold ? (
                <span>
                  ※ 認証処理中です（完了後にこの画面で10秒表示します）。
                </span>
              ) : effectivePhase === "first" ? (
                <span>
                  ※
                  10秒後にログインページへ自動で移動します（すぐ進む場合は「続ける」）。
                </span>
              ) : effectivePhase === "second_pending" ? (
                <span>
                  ※ この画面は待機します。届いたメールを確認してクリックして、
                  <br />
                  その後<b>”Accept”</b>を押してログインしてください。
                </span>
              ) : (
                <span>※ 自動で進まない場合は「続ける」を押してください。</span>
              )}
            </div>
          </>
        ) : (
          <div className={styles.progressWrap}>
            <div className={styles.progressLoop}>
              <div className={styles.barLoop} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
