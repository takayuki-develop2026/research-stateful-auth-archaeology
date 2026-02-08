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
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --------------------
  // ✅ effectivePhase: “pが無い /auth/callback” を first として救済
  // --------------------
  const effectivePhase: FlowPhase | null = useMemo(() => {
    if (phaseFromQuerySafe) return phaseFromQuerySafe;

    // code がある場合は OIDC callback → IdaasProvider が hold に飛ばすのが正
    if (code) return null;

    // error は login へ戻すUIにしたいなら null のままでOK
    if (error) return null;

    // ✅ それ以外で /auth/callback に来た = verify link戻りを first として救済
    return "first";
  }, [phaseFromQuerySafe, code, error]);

  // --------------------
  // ✅ phase の SoT: p があれば採用。p無しだけど effectivePhase=first なら holdへ正規化
  // --------------------
  useEffect(() => {
    if (!hydrated) return;

    // p があるならそれをSoTにする
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

    // ✅ p無し救済：effectivePhase=first なら URL を hold に正規化して「p付き」にする
    if (!code && !error && effectivePhase === "first") {
      const hold = buildHoldUrl({
        phase: "first",
        finalReturnTo: returnToRaw,
        nonce: null,
      });

      d("phase:repair_no_p_no_code", {
        action: "router.replace(hold)",
        to: hold,
        returnToRaw,
      });

      router.replace(hold);
      return;
    }

    // p無し + codeあり は何もしない（IdaasProvider が hold に飛ばす）
    d("phase:no_p", {
      hasCode: !!code,
      hasError: !!error,
      effectivePhase,
      note: "waiting for IdaasProvider to redirect to hold",
    });
  }, [
    hydrated,
    phaseFromQuerySafe,
    nonceFromQuery,
    code,
    error,
    effectivePhase,
    returnToRaw,
    router,
  ]);

  // --------------------
  // screen/duration: effectivePhase 基準
  // --------------------
  const screen: Screen = useMemo(() => {
    if (effectivePhase === "first") return "verify";
    if (effectivePhase === "second_pending") return "verify";
    if (effectivePhase === "second_done") return "verify_done";
    return "login";
  }, [effectivePhase]);

  const isVerify = screen === "verify" || screen === "verify_done";

  const durationMs = useMemo(() => {
    if (effectivePhase === "second_done") return STEP2_MS;
    if (effectivePhase === "first" || effectivePhase === "second_pending")
      return STEP1_MS;
    return 0;
  }, [effectivePhase]);

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
    if (effectivePhase === "second_done") return "メール認証 2/2";
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
    if (onceRef.current) return;
    onceRef.current = true;

    const nonce = genNonce();
    setNonceToSession(nonce);

    const holdPending = buildHoldUrl({
      phase: "second_pending",
      finalReturnTo: returnToRaw,
      nonce,
    });

    // ✅ AuthProvider が callback 完了後に読む returnTo を hold に固定
    setReturnToToSession(holdPending);

    // 画面側も合わせる
    writeFlow("second_pending");
    setPhase("second_pending");

    d("action:goToAuth0SignupForSecond", {
      nonce,
      holdPending,
      returnToRaw,
    });

    try {
      await login({ type: "oidc", returnTo: holdPending });
    } catch (e) {
      console.error(e);
      router.replace(`/login?oidc_error=1`);
    }
  }, [login, router, returnToRaw]);

  /**
   * ✅ second_pending になったら、次の callback 後の着地点を “second_done hold” に仕込む
   */
  useEffect(() => {
    if (!hydrated) return;
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
  }, [hydrated, effectivePhase, returnToRaw]);

  /**
   * ✅ 2/2 完走後 → 最終ログイン（callback後は final returnTo へ）
   */
  const finalizeAfterSecondDone = useCallback(async () => {
    if (onceRef.current) return;
    onceRef.current = true;

    // flow掃除
    clearFlow();
    clearNonceFromSession();
    setPhase(null);

    d("action:finalizeAfterSecondDone", { returnToRaw });

    // ✅ ここでme同期だけ取って終わる（Auth0へ戻らない）
    let me: any = null;
    try {
      me = await refresh?.();
    } catch {}

    const next = resolvePostVerifyReturnTo(me, returnToRaw);
    router.replace(next || "/");
  }, [refresh, router, returnToRaw]);

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

        d("story:ready", { effectivePhase });

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
  ]);

  // --------------------
  // continue
  // --------------------
  const onContinue = useCallback(async () => {
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

      // ✅ 次の callback 後の着地点は second_done
      setReturnToToSession(holdDone);

      d("ui:continue_second_pending_to_done", { nonce, holdDone });

      try {
        await login({ type: "oidc", returnTo: holdDone }); // ✅ ここが重要
      } catch (e) {
        console.error(e);
        router.replace(`/login?oidc_error=1`);
      }
      return;
    }

  }, [
    effectivePhase,
    returnToRaw,
    goToAuth0SignupForSecond,
    finalizeAfterSecondDone,
    login,
    router,
  ]);

  // --------------------
  // hydration guard
  // --------------------
  if (!hydrated) {
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
  // ✅ render
  // - 1回目/2回目/3回目：全部同じS1アニメ（envelope+plane+sparks）
  // - テキストだけ phase で差し替え
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

          {/* ✅ verify/verify_done/second_pending すべて同じ S1 */}
          {isVerify ? (
            <div className={`${styles.scene} ${styles.sceneS1}`}>
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
            </div>
          ) : null}

          {/* login時 */}
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
                <div className={styles.instLead}>
                  メール認証 2/2 登録の手順
                </div>
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
