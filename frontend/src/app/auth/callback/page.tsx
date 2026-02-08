"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";
import styles from "./W-AuthCallbackPage.module.css";

type Screen = "login" | "verify" | "verify_done";
type EmailStage = "first" | "second";

const STEP1_MS = 10_000;
const STEP2_MS = 10_000;
const VERIFY_DONE_HOLD_MS = 2_000;
const TICK_MS = 100;

const STAGE_KEY = "occore_email_stage_v1";

function d(...args: any[]) {
  console.log("[AuthCallback]", ...args);
}

function clamp(n: number, a = 0, b = 1) {
  return Math.max(a, Math.min(b, n));
}

function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

// ✅ localStorage（別タブでも保持される）
function safeGetStage(): EmailStage | null {
  try {
    const v = localStorage.getItem(STAGE_KEY);
    if (v === "first" || v === "second") return v;
    return null;
  } catch {
    return null;
  }
}

function safeSetStage(v: EmailStage) {
  try {
    localStorage.setItem(STAGE_KEY, v);
  } catch {}
}

function safeClearStage() {
  try {
    localStorage.removeItem(STAGE_KEY);
  } catch {}
}

export default function AuthCallbackPage() {
  const sp = useSearchParams();
  const router = useRouter();
  const { login } = useAuth();

  const screenRaw = (sp.get("screen") as Screen | null) ?? null;
  const returnToRaw = sp.get("returnTo") ?? "/";

  const code = sp.get("code");
  const error = sp.get("error");

  const [emailStage, setEmailStage] = useState<EmailStage | null>(null);
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => {
    setHydrated(true);
    setEmailStage(safeGetStage());
  }, []);

  /**
   * ✅ 絶対に崩れない “screen決定” ルール
   *
   * - 2/2(verify_done) は emailStage === "second" のときだけ許可
   * - stage が無い（メールから別タブ起動/初回など）場合は verify 系は必ず 1/2 に倒す
   *   → 「1回目なのに returnTo=verify-second」事故を確実に吸収
   */
  const screen: Screen = useMemo(() => {
    // 2/2 を出していいのは “自分が second をセットした” 場合だけ
    if (emailStage === "second") return "verify_done";
    if (emailStage === "first") return "verify";

    const wantsVerifyFlow =
      returnToRaw.startsWith("/email/verify-") ||
      screenRaw === "verify" ||
      screenRaw === "verify_done";

    if (wantsVerifyFlow) {
      // ✅ stage無しで verify 系なら必ず 1/2
      return "verify";
    }

    if (screenRaw === "login") return "login";
    return "login";
  }, [emailStage, returnToRaw, screenRaw]);

  const returnTo = returnToRaw;

  // 1/2完了後は 2/2登録へ
  const nextReturnToForVerify = "/email/verify-second";

  const isVerify = screen === "verify" || screen === "verify_done";

  const durationMs = useMemo(() => {
    if (screen === "verify") return STEP1_MS;
    if (screen === "verify_done") return STEP2_MS;
    return 0;
  }, [screen]);

  const rootRef = useRef<HTMLDivElement | null>(null);
  const storyRef = useRef<HTMLDivElement | null>(null);

  const readyRef = useRef(false);
  const phaseRef = useRef<"idle" | "story" | "logging_in">(isVerify ? "story" : "idle");
  const startedAtRef = useRef<number>(0);

  const onceRef = useRef(false);

  const rafRef = useRef<number | null>(null);
  const tickTimerRef = useRef<number | null>(null);
  const afterReadyTimerRef = useRef<number | null>(null);

  const stepLabel = useMemo(() => {
    if (screen === "verify") return "STEP 1/2";
    if (screen === "verify_done") return "STEP 2/2";
    return "AUTH";
  }, [screen]);

  const title = useMemo(() => {
    if (screen === "verify") return "メール確認 1/2";
    if (screen === "verify_done") return "メール確認 2/2";
    return "Signing in…";
  }, [screen]);

  const baseMessage = useMemo(() => {
    if (screen === "verify") {
      return "メール認証 1/2 が完了しました。次は 2/2 の登録へ進みます。";
    }
    if (screen === "verify_done") {
      return "メール認証 2/2 が完了しました。10秒後にログイン処理を開始します。";
    }
    if (code) return "認証情報を受け取りました。トークン交換中…";
    if (error) return "SSOでエラーが発生しました。ログイン画面に戻ります…";
    return "SSO認証を開始しています。";
  }, [screen, code, error]);

  const qs = <T extends Element>(sel: string) => {
    const el = rootRef.current;
    if (!el) return null;
    return el.querySelector<T>(sel);
  };

  const setCssVar = (name: string, value: string) => {
    const el = storyRef.current ?? rootRef.current;
    if (!el) return;
    (el as HTMLElement).style.setProperty(name, value);
  };

  const setDomReady = (v: boolean) => {
    const el = rootRef.current;
    if (!el) return;
    el.dataset.ready = v ? "1" : "0";
  };

  const setDomPhase = (p: "idle" | "story" | "logging_in") => {
    const el = rootRef.current;
    if (!el) return;
    el.dataset.phase = p;
  };

  const setDomSecondsLeft = (sec: number) => {
    const t = qs<HTMLElement>(`[data-el="secondsLeft"]`);
    if (t) t.textContent = String(sec);
  };

  const setDomHintText = (text: string) => {
    const t = qs<HTMLElement>(`[data-el="hintText"]`);
    if (t) t.textContent = text;
  };

  const setDomStatusText = (text: string) => {
    const t = qs<HTMLElement>(`[data-el="statusText"]`);
    if (t) t.textContent = text;
  };

  const setDomMessageText = (text: string) => {
    const t = qs<HTMLElement>(`[data-el="messageText"]`);
    if (t) t.textContent = text;
  };

  const setDomButtonDisabled = (disabled: boolean) => {
    const btn = qs<HTMLButtonElement>(`[data-el="continueBtn"]`);
    if (!btn) return;
    btn.disabled = disabled;
    btn.setAttribute("aria-disabled", disabled ? "true" : "false");
    btn.classList.toggle(styles.btnDisabled, disabled);
  };

  const clearTimers = () => {
    if (rafRef.current) cancelAnimationFrame(rafRef.current);
    rafRef.current = null;

    if (tickTimerRef.current) window.clearInterval(tickTimerRef.current);
    tickTimerRef.current = null;

    if (afterReadyTimerRef.current) window.clearTimeout(afterReadyTimerRef.current);
    afterReadyTimerRef.current = null;
  };

  const goToAuth0LoginForSecondSignup = async () => {
    if (onceRef.current) return;
    onceRef.current = true;

    phaseRef.current = "logging_in";
    setDomPhase("logging_in");
    setDomStatusText("WORKING");
    setDomHintText("ログインページへ移動中…");
    setDomMessageText("Auth0 ログインページへ移動します…");
    setDomButtonDisabled(true);

    // ✅ 2/2 を表示できる唯一の条件をここで作る
    safeSetStage("second");
    setEmailStage("second");

    sessionStorage.setItem("occore_return_to_v1", nextReturnToForVerify);

    try {
      await login({ type: "oidc", returnTo: nextReturnToForVerify });
    } catch (e) {
      console.error("🔥DEBUG goToAuth0LoginForSecondSignup failed", e);
      router.replace(`/login?oidc_error=1`);
    }
  };

  const autoLoginAfterVerifyDone = async () => {
    if (onceRef.current) return;
    onceRef.current = true;

    phaseRef.current = "logging_in";
    setDomPhase("logging_in");
    setDomStatusText("WORKING");
    setDomHintText("ログイン処理中…");
    setDomMessageText("ログイン処理中… 少々お待ちください。");
    setDomButtonDisabled(true);

    sessionStorage.setItem("occore_return_to_v1", returnTo);

    await sleep(VERIFY_DONE_HOLD_MS);

    // ✅ 2/2完了 → stage消す
    safeClearStage();
    setEmailStage(null);

    try {
      await login({ type: "oidc", returnTo });
    } catch (e) {
      console.error("🔥DEBUG auto login(verify_done) failed", e);
      router.replace(`/login?oidc_error=1`);
    }
  };

  const startStory = () => {
    if (!isVerify) return;

    const dur = durationMs;
    const start = performance.now();
    startedAtRef.current = start;

    setCssVar("--p", "0");
    setDomSecondsLeft(Math.ceil(dur / 1000));
    setDomHintText("続けるまで");
    setDomStatusText("PREPARING");
    setDomMessageText(baseMessage);

    tickTimerRef.current = window.setInterval(() => {
      const now = performance.now();
      const elapsed = now - startedAtRef.current;
      const left = Math.max(0, Math.ceil((dur - elapsed) / 1000));
      setDomSecondsLeft(left);
    }, TICK_MS);

    const tick = (now: number) => {
      const elapsed = now - start;
      const p = clamp(elapsed / dur, 0, 1);

      setCssVar("--p", String(p));

      if (p >= 1) {
        if (tickTimerRef.current) {
          window.clearInterval(tickTimerRef.current);
          tickTimerRef.current = null;
        }

        readyRef.current = true;
        setDomReady(true);
        setDomButtonDisabled(false);
        setDomStatusText("READY");
        setDomHintText("準備完了");

        rafRef.current = null;

        afterReadyTimerRef.current = window.setTimeout(() => {
          if (screen === "verify") void goToAuth0LoginForSecondSignup();
          if (screen === "verify_done") void autoLoginAfterVerifyDone();
        }, 0);

        return;
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
  };

  useEffect(() => {
    clearTimers();

    readyRef.current = false;
    phaseRef.current = isVerify ? "story" : "idle";
    startedAtRef.current = performance.now();
    onceRef.current = false;

    setDomReady(false);
    setDomPhase(phaseRef.current);

    setDomButtonDisabled(isVerify);
    setDomStatusText(isVerify ? "PREPARING" : "WORKING");
    setDomMessageText(baseMessage);

    setCssVar("--dur", `${durationMs}ms`);
    setCssVar("--p", "0");

    d("screen init", {
      hydrated,
      emailStage,
      screenRaw,
      returnToRaw,
      screen,
      isVerify,
      durationMs,
      returnTo,
    });

    if (isVerify) {
      startStory();
      return () => clearTimers();
    }

    setDomHintText("");
    setDomSecondsLeft(0);

    return () => clearTimers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hydrated, emailStage, screen, isVerify, durationMs, returnTo, baseMessage]);

  useEffect(() => {
    if (isVerify) return;

    if (code || error) {
      if (error) router.replace(`/login?oidc_error=1`);
      return;
    }

    if (screen !== "login") return;
    if (onceRef.current) return;
    onceRef.current = true;

    login({ type: "oidc", returnTo }).catch((e: any) => {
      console.error("🔥DEBUG login(oidc) failed", e);
      router.replace(`/login?oidc_error=1`);
    });
  }, [screen, isVerify, code, error, login, returnTo, router]);

  const onContinue = async () => {
    if (!readyRef.current) return;

    if (screen === "verify") {
      await goToAuth0LoginForSecondSignup();
      return;
    }

    if (screen === "verify_done") {
      await autoLoginAfterVerifyDone();
      return;
    }

    sessionStorage.setItem("occore_return_to_v1", returnTo);
    try {
      await login({ type: "oidc", returnTo });
    } catch (e) {
      console.error(e);
      router.replace(`/login?oidc_error=1`);
    }
  };

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
              {isVerify ? "PREPARING" : "WORKING"}
            </div>
            <div className={styles.spinner} aria-hidden="true" />
          </div>
        </div>


        {/* ========= Story ========= */}
        <div
          ref={storyRef}
          className={styles.story}
          aria-hidden="true"
          style={{
            ["--dur" as any]: `${durationMs}ms`,
            ["--p" as any]: 0,
          }}
        >
          <div className={styles.halo} />
          <div className={styles.halo2} />

          {screen === "verify" ? (
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

          {screen === "verify_done" ? (
            <div className={`${styles.scene} ${styles.sceneS2}`}>
              <div className={styles.scanOnce} />

              <div className={styles.ringOnce}>
                <div className={styles.ringSeg} />
                <div className={styles.ringInner} />
                <div className={styles.check}>
                  <div className={styles.checkLeft} />
                  <div className={styles.checkRight} />
                </div>
              </div>

              <div className={styles.pulseOnce} />
              <div className={styles.pulseOnce2} />
            </div>
          ) : null}

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

        {screen === "verify" ? (
          <div className={styles.instructions}>
            <div className={styles.instTitle}>次の手順</div>

            <div className={styles.instBlock}>
              <div className={styles.instLead}>
                メール認証 1/2 完了しました。
              </div>
              <div className={styles.instText}>
                ログインページの <b>”Continue”</b> の下の <b>”Sign up”</b>{" "}
                を押して
                <br />
                メールとパスワードを入力して <b>メール認証 2/2</b>{" "}
                の登録してください。
              </div>
            </div>

            <div className={styles.instBlock}>
              <div className={styles.instLead}>メール認証 2/2 登録後の手順</div>
              <ol className={styles.instList}>
                <li>
                  <b>“OmniCommerce Core CLI_Native”</b> からメールが届きます
                </li>
                <li>
                  メールAccountを確認して、<b>“Verify Link”</b> または{" "}
                  <b>“Verify Your Account”</b> をクリックしてください。
                </li>
              </ol>
            </div>
          </div>
        ) : null}

        {isVerify ? (
          <>
            <div className={styles.progressWrap}>
              <div className={styles.progress}>
                <div className={styles.barOnce} />
                <div className={styles.glintOnce} />
              </div>

              <div className={styles.hint}>
                <span data-el="hintText">続けるまで</span>{" "}
                <b data-el="secondsLeft">10</b> 秒
              </div>
            </div>

            <button
              data-el="continueBtn"
              onClick={onContinue}
              disabled
              className={`${styles.btn} ${styles.btnDisabled}`}
            >
              <span className={styles.btnGlow} aria-hidden="true" />
              続ける
            </button>

            <div className={styles.sub}>
              {screen === "verify" ? (
                <span>
                  ※
                  10秒後にログインページへ自動で移動します（すぐ進む場合は「続ける」）。
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
