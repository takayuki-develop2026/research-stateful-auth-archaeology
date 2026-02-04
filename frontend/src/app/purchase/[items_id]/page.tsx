"use client";

// import AdyenCheckout from "@adyen/adyen-web";
// import { AdyenCheckout, Dropin } from "@adyen/adyen-web";
import "@adyen/adyen-web/styles/adyen.css";

import React, { useMemo, useState, useEffect, useRef } from "react";
import { useParams, useRouter } from "next/navigation";

import { useAuth } from "@/ui/auth/AuthProvider";
import { useItemDetailSWR } from "@/services/useItemDetailSWR";
import { useUserPrimaryAddressSWR } from "@/services/useUserPrimaryAddressSWR";
import { getImageUrl } from "@/utils/utils";
import styles from "./W-Purchase-Confirm.module.css";

import { loadStripe } from "@stripe/stripe-js";
import {
  Elements,
  CardElement,
  useStripe,
  useElements,
} from "@stripe/react-stripe-js";

/* ================= Stripe ================= */
const stripePromise = loadStripe(
  process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY!,
  { locale: "ja" },
);

type PaymentMethod = "" | "card" | "konbini";

/* ================= Wrapper ================= */
export default function PurchaseConfirmPageWrapper() {
  return (
    <Elements
      stripe={stripePromise}
      options={{
        locale: "ja",
        appearance: { theme: "stripe" },
      }}
    >
      <PurchaseConfirmPage />
    </Elements>
  );
}

const loadAdyenSdk = async (): Promise<{
  AdyenCheckout: (config: any) => Promise<any>;
  Dropin: any;
  Card: any;
}> => {
  const mod: any = await import("@adyen/adyen-web");

  const fn = mod?.AdyenCheckout ?? mod?.createCheckout ?? mod?.default ?? mod;

  if (typeof fn !== "function") {
    console.error("[Adyen] module shape:", mod);
    throw new Error("AdyenCheckout function not found");
  }

  const Dropin = mod?.Dropin ?? mod?.default?.Dropin;
  if (!Dropin) throw new Error("Dropin not found");

  // ★追加：Card
  const Card = mod?.Card ?? mod?.default?.Card;
  if (!Card) {
    console.error("[Adyen] module shape:", mod);
    throw new Error("Card component not found");
  }

  return { AdyenCheckout: fn, Dropin, Card };
};

type CreateOrderResponse = { order_id: number };

type StartPaymentResponse =
  | { provider: "stripe"; client_secret: string }
  | {
      provider: "adyen";
      session_id: string;
      session_data: string;
      client_key?: string; // backend互換（使わない）
      environment: "test" | "live";
    };

type OneClickResponse = {
  payment_id: number;
  status: string;
  provider_payment_id: string;
  client_secret: string | null;
  requires_action: boolean;
};

type WalletPaymentMethodsResponse = {
  exists: boolean;
  payment_methods: Array<{
    id: number;
    provider: string;
    provider_payment_method_id: string;
    source: string; // "card"
    brand: string;
    last4: string;
    exp_month: number;
    exp_year: number;
    is_default: boolean;
    one_click_eligible: boolean;
  }>;
};

type CreateSetupIntentResponse = {
  setup_intent_id: string;
  client_secret: string;
};

// ✅ Adyen Drop-in 表示用 state
type AdyenSessionState = {
  orderId: number;
  sessionId: string;
  sessionData: string;
  clientKey: string;
  environment: "test" | "live";
};

function PurchaseConfirmPage() {
  const router = useRouter();
  const params = useParams();
  const { apiClient, isAuthenticated, isLoading: isAuthLoading } = useAuth();

  const stripe = useStripe();
  const elements = useElements();

  const itemId = useMemo(() => {
    const raw = (params as any).items_id;
    const n = Number(raw);
    return Number.isNaN(n) ? null : n;
  }, [params]);

  const {
    item,
    isLoading: isItemLoading,
    isError: isItemError,
  } = useItemDetailSWR(itemId);

  const {
    address,
    isLoading: isAddressLoading,
    isError: isAddressError,
  } = useUserPrimaryAddressSWR();

  const [payment, setPayment] = useState<PaymentMethod>("");
  const [processing, setProcessing] = useState(false);

  // --- One-click UI state ---
  const [walletLoading, setWalletLoading] = useState(false);
  const [oneClickAvailable, setOneClickAvailable] = useState(false);
  const [oneClickEnabled, setOneClickEnabled] = useState(false);
  const [defaultPmLabel, setDefaultPmLabel] = useState<string | null>(null);

  // --- Save-card state ---
  const [saveCardLoading, setSaveCardLoading] = useState(false);

  // --- Adyen state ---
  const [adyenSession, setAdyenSession] = useState<AdyenSessionState | null>(
    null,
  );
  const adyenContainerRef = useRef<HTMLDivElement | null>(null);

  // ✅ StrictMode二重useEffect対策：同一sessionIdでは初期化しない
  const initializedSessionIdRef = useRef<string | null>(null);

  // env: clientKey（フロント用）
  const ADYEN_CLIENT_KEY = (
    process.env.NEXT_PUBLIC_ADYEN_CLIENT_KEY ?? ""
  ).trim();

  const safeStringifyError = (e: any) => {
    try {
      if (!e) return "null";
      if (typeof e === "string") return e;
      if (e instanceof Error)
        return `${e.name}: ${e.message}\n${e.stack ?? ""}`;
      return JSON.stringify(e, Object.getOwnPropertyNames(e), 2);
    } catch {
      return String(e);
    }
  };

  const applyWalletState = (
    res: WalletPaymentMethodsResponse | null | undefined,
  ) => {
    const list = res?.payment_methods ?? [];
    const def = list.find((x) => x.is_default);

    const ok =
      res?.exists === true &&
      !!def &&
      def.source === "card" &&
      def.one_click_eligible === true;

    setOneClickAvailable(ok);

    if (ok && def) {
      setDefaultPmLabel(
        `${String(def.brand ?? "").toUpperCase()} **** ${def.last4} (exp ${def.exp_month}/${def.exp_year})`,
      );
      setOneClickEnabled(true);
    } else {
      setDefaultPmLabel(null);
      setOneClickEnabled(false);
    }
  };

  // card 選択時だけ wallet を確認
  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      setOneClickAvailable(false);
      setDefaultPmLabel(null);
      setOneClickEnabled(false);

      if (!apiClient) return;
      if (!isAuthenticated) return;
      if (payment !== "card") return;

      try {
        setWalletLoading(true);
        const res = await apiClient.get<WalletPaymentMethodsResponse>(
          "/wallet/payment-methods",
        );
        if (cancelled) return;
        applyWalletState(res);
      } catch (e) {
        if (cancelled) return;
        console.error("[🔥wallet/payment-methods] failed", e);
        setOneClickAvailable(false);
        setOneClickEnabled(false);
        setDefaultPmLabel(null);
      } finally {
        if (!cancelled) setWalletLoading(false);
      }
    };

    run();
    return () => {
      cancelled = true;
    };
  }, [payment, apiClient, isAuthenticated]);

  const waitUntilPaid = async (orderId: number, timeoutMs = 15000) => {
    if (!apiClient) return false;

    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      try {
        const detail = await apiClient.get<any>(`/me/orders/${orderId}`);
        if (detail?.order_status === "paid") return true;
      } catch {
        // ignore
      }
      await new Promise((r) => setTimeout(r, 700));
    }
    return false;
  };

  // ✅ Adyen Drop-in mount（@adyen/adyen-web）
  useEffect(() => {
    if (!adyenSession) return;
    if (!adyenContainerRef.current) return;
// ✅ StrictMode二重useEffect対策：同一sessionIdでは初期化しない
if (initializedSessionIdRef.current === adyenSession.sessionId) return;
initializedSessionIdRef.current = adyenSession.sessionId;
    let cancelled = false;
    let dropin: any = null;

    (async () => {
      try {
        const el = adyenContainerRef.current!;
        el.innerHTML = "";

        // ✅ 見えない事故（高さ0/親のoverflow）を潰す保険
        el.style.minHeight = "420px";

        console.log("[AdyenDropin] init start", {
          orderId: adyenSession.orderId,
          env: adyenSession.environment,
          sessionId: adyenSession.sessionId,
          sessionDataLen: adyenSession.sessionData.length,
          clientKeyLen: adyenSession.clientKey.length,
        });

        const { AdyenCheckout, Dropin, Card } = await loadAdyenSdk();

        const checkout = await AdyenCheckout({
          environment: adyenSession.environment,
          clientKey: adyenSession.clientKey,
          session: {
            id: adyenSession.sessionId,
            sessionData: adyenSession.sessionData,
          },
          locale: "ja-JP",
          countryCode: "JP",
          amount: { currency: "JPY", value: resolvedItem.price }, // ★必須寄り（minor units）

          onPaymentCompleted: async (result: any) => {
            console.log("[Adyen] onPaymentCompleted", result);
            if (cancelled) return;

            const ok = await waitUntilPaid(adyenSession.orderId, 20000);
            console.log("[Adyen] waitUntilPaid", { ok });

            router.replace(
              `/thanks/buy/adyen?order_id=${adyenSession.orderId}&paid=${ok ? 1 : 0}`,
            );
          },

          onError: (err: any) => {
            console.error("[Adyen] onError", err);
            if (cancelled) return;

            alert(
              "Adyen決済でエラーが発生しました。\n\n" + safeStringifyError(err),
            );
            setProcessing(false);
            setAdyenSession(null);
          },
        });

        // ★重要：scheme(カード)を描画するCardコンポーネントを登録
        dropin = new Dropin(checkout, {
          showPayButton: true,
          openFirstPaymentMethod: true,
          paymentMethodComponents: [Card], // ← これがないと scheme が出ない

          // ここは card でOK（Adyenの設定名）
          paymentMethodsConfiguration: {
            card: {
              hasHolderName: true,
              holderNameRequired: true,
              hideCVC: false,
            },
          },
        });

        dropin.mount(el);

        console.log(
          "[Adyen] typeof checkout.create",
          typeof (checkout as any).create,
        );
        console.log(
          "[Adyen] checkout.paymentMethodsResponse",
          checkout?.paymentMethodsResponse,
        );
        console.log(
          "[Adyen] paymentMethods types",
          checkout?.paymentMethodsResponse?.paymentMethods?.map(
            (pm: any) => pm.type,
          ) ?? [],
        );

        if (cancelled) return;

        // ✅ セッションフローは「Dropinクラスで作る」のが安定（checkout.create 依存しない）
        dropin = new Dropin(checkout, {
          showPayButton: true,
          openFirstPaymentMethod: true, // あると安定
          paymentMethodsConfiguration: {
            card: {
              hasHolderName: true,
              holderNameRequired: true,
              hideCVC: false,
            },
          },
        });
        dropin.mount(el);

        const dump = (label: string) => {
          const iframes = el.querySelectorAll("iframe");
          const rect = el.getBoundingClientRect();
          const cs = window.getComputedStyle(el);
          console.log(`[AdyenDump] ${label}`, {
            iframeCount: iframes.length,
            rect: { w: rect.width, h: rect.height },
            style: {
              display: cs.display,
              visibility: cs.visibility,
              opacity: cs.opacity,
              overflow: cs.overflow,
              minHeight: cs.minHeight,
            },
            htmlLen: el.innerHTML.length,
          });
        };

        dump("after-mount:0ms");
        setTimeout(() => dump("after-mount:300ms"), 300);
        setTimeout(() => dump("after-mount:1500ms"), 1500);

        console.log("[AdyenDropin] mounted OK");
        setProcessing(false);
      } catch (e) {
        console.error("[AdyenDropin] init failed", e);
        alert("Adyen決済の初期化に失敗しました。\n\n" + safeStringifyError(e));
        setProcessing(false);
        setAdyenSession(null);
      }
    })();

    return () => {
      cancelled = true;
      try {
        dropin?.unmount?.();
      } catch {}
    };
  }, [adyenSession, router]);

  // ✅ 保存カード（SetupIntent → confirmCardSetup）
  const saveCardForOneClick = async () => {
    if (!apiClient) {
      alert("APIクライアントが準備できていません。");
      return;
    }
    if (!isAuthenticated) {
      alert("ログインが必要です。");
      return;
    }
    if (!stripe || !elements) {
      alert("決済の準備が整っていません。");
      return;
    }

    const card = elements.getElement(CardElement);
    if (!card) {
      alert("カード入力欄が見つかりません。");
      return;
    }

    try {
      setSaveCardLoading(true);

      const si = await apiClient.post<CreateSetupIntentResponse>(
        "/wallet/setup-intent",
        {},
      );
      if (!si?.client_secret) {
        alert("client_secret が取得できませんでした。");
        return;
      }

      const result = await stripe.confirmCardSetup(si.client_secret, {
        payment_method: { card },
      });
      if (result.error) {
        alert(result.error.message ?? "カード保存に失敗しました。");
        return;
      }

      const status = result.setupIntent?.status;
      if (status !== "succeeded") {
        alert("カード保存が完了しませんでした（status != succeeded）");
        return;
      }

      await new Promise((r) => setTimeout(r, 1200));

      setWalletLoading(true);
      const wallet = await apiClient.get<WalletPaymentMethodsResponse>(
        "/wallet/payment-methods",
      );
      applyWalletState(wallet);

      alert("カードを保存しました。One-clickが利用可能になります。");
    } catch (e: any) {
      console.error("[🔥saveCardForOneClick] failed", e);
      alert(
        e?.response?.data?.message ?? e?.message ?? "カード保存に失敗しました",
      );
    } finally {
      setWalletLoading(false);
      setSaveCardLoading(false);
    }
  };

  /* ================= Guard ================= */
  if (isAuthLoading || isItemLoading || isAddressLoading) {
    return <div className={styles.loadingOverlay}>購入情報を読み込み中...</div>;
  }

  if (isItemError || isAddressError) {
    return (
      <div className={styles.loadingOverlay}>
        情報の取得に失敗しました。時間をおいて再度お試しください。
      </div>
    );
  }

  if (!item) {
    return (
      <div className={styles.loadingOverlay}>購入情報を準備しています...</div>
    );
  }

  const resolvedItem = item;

  const isAdyenFlow = adyenSession !== null;
  const oneClickUiEnabled = !isAdyenFlow;

  const needsCardInput =
    payment === "card" &&
    !isAdyenFlow &&
    (!oneClickAvailable || !oneClickEnabled);

  const canPurchase =
    isAuthenticated &&
    resolvedItem.remain > 0 &&
    payment !== "" &&
    !!address?.id &&
    !processing &&
    !isAdyenFlow;

  /* ================= submit ================= */
  const submitPurchase = async () => {
    if (!canPurchase || !apiClient || !address) return;

    try {
      setProcessing(true);

      // ① Order 作成
      const orderRes = await apiClient.post<CreateOrderResponse>("/orders", {
        shop_id: resolvedItem.shop_id,
        items: [
          {
            item_id: resolvedItem.id,
            name: resolvedItem.name,
            price_amount: resolvedItem.price,
            price_currency: "JPY",
            quantity: 1,
            image_path: resolvedItem.item_image,
          },
        ],
      });

      const orderId = orderRes.order_id;

      // ② 配送先確定
      await apiClient.post(`/orders/${orderId}/address`, {
        address_id: address.id,
      });

      // ③ Order 確定
      await apiClient.post(`/orders/${orderId}/confirm`);

      // ④ Payment
      // ---- One-click (Stripe) ----
      if (
        payment === "card" &&
        oneClickUiEnabled &&
        oneClickEnabled &&
        oneClickAvailable
      ) {
        const oc = await apiClient.post<OneClickResponse>(
          "/wallet/one-click-checkout",
          { order_id: orderId },
        );

        if (oc.requires_action) {
          if (!stripe) {
            alert("決済の準備が整っていません。");
            setProcessing(false);
            return;
          }
          if (!oc.client_secret) {
            alert("client_secret が取得できませんでした。");
            setProcessing(false);
            return;
          }
          const result = await stripe.confirmCardPayment(oc.client_secret);
          if (result.error) {
            alert(result.error.message);
            setProcessing(false);
            return;
          }
        }

        await waitUntilPaid(orderId);
        router.replace(`/thanks/buy/stripe-card?order_id=${orderId}`);
        return;
      }

      // ---- 通常 ----
      const paymentRes = await apiClient.post<StartPaymentResponse>(
        "/payments/start",
        {
          order_id: orderId,
          method: payment,
        },
      );

      console.log("[🔥/payments/start]", paymentRes);
      console.log(
        "[🔥ADYEN_CLIENT_KEY env]",
        process.env.NEXT_PUBLIC_ADYEN_CLIENT_KEY,
      );

      if (payment !== "card") {
        router.replace(`/thanks/buy/konbini?order_id=${orderId}`);
        return;
      }

      // Stripe
      if (paymentRes.provider === "stripe") {
        if (!stripe || !elements) {
          alert("決済の準備が整っていません。");
          setProcessing(false);
          return;
        }

        const card = elements.getElement(CardElement);
        if (!card) {
          alert("カード入力欄が見つかりません。");
          setProcessing(false);
          return;
        }

        const result = await stripe.confirmCardPayment(
          paymentRes.client_secret,
          {
            payment_method: { card },
          },
        );

        if (result.error) {
          alert(result.error.message);
          setProcessing(false);
          return;
        }

        await waitUntilPaid(orderId);
        router.replace(`/thanks/buy/stripe-card?order_id=${orderId}`);
        return;
      }

      // Adyen
      if (!ADYEN_CLIENT_KEY) {
        alert("Adyen clientKey が未設定です（NEXT_PUBLIC_ADYEN_CLIENT_KEY）。");
        setProcessing(false);
        return;
      }

      console.log("[Adyen branch] start", {
        envKey: ADYEN_CLIENT_KEY,
        session_id: paymentRes.session_id,
        environment: paymentRes.environment,
        session_data_len: paymentRes.session_data.length,
      });

      setAdyenSession({
        orderId,
        sessionId: paymentRes.session_id,
        sessionData: paymentRes.session_data,
        clientKey: ADYEN_CLIENT_KEY,
        environment: paymentRes.environment,
      });

      // ✅ ここで必ず落とす（「処理中に見える」を根絶）
      setProcessing(false);

      console.log("[Adyen branch] setAdyenSession called");
      return;
    } catch (e: any) {
      console.error(e);
      alert(
        e?.response?.data?.message ?? e?.message ?? "購入処理に失敗しました",
      );
      setProcessing(false);
    }
  };

  /* ================= JSX ================= */
  return (
    <div className={styles.item_buy_wrapper}>
      <div className={styles.item_buy_contents}>
        <div className={styles.item_buy_lr}>
          {/* LEFT */}
          <div className={styles.item_buy_l}>
            <div className={styles.item_buy_content_section}>
              <div className={styles.item_buy_image}>
                <img
                  src={getImageUrl(resolvedItem.item_image)}
                  alt={resolvedItem.name}
                />
              </div>
              <div>
                <h3 className={styles.item_name}>{resolvedItem.name}</h3>
                <p className={styles.item_price}>
                  ¥{resolvedItem.price.toLocaleString()}
                </p>
              </div>
            </div>

            <div className={styles.item_buy_content_section}>
              <h4>支払い方法</h4>
              <select
                value={payment}
                onChange={(e) => setPayment(e.target.value as PaymentMethod)}
                disabled={processing || isAdyenFlow}
              >
                <option value="">選択してください</option>
                <option value="konbini">コンビニ支払い</option>
                <option value="card">クレジットカード支払い</option>
              </select>

              {/* ✅ DEBUG panel（原因確定したら消してOK） */}
              <div
                style={{
                  marginTop: 10,
                  padding: 8,
                  border: "1px solid #e5e7eb",
                  background: "#fffbe6",
                }}
              >
                <div>debug:isAdyenFlow = {String(isAdyenFlow)}</div>
                <div>debug:processing = {String(processing)}</div>
                <div>
                  debug:ADYEN_CLIENT_KEY(len) = {ADYEN_CLIENT_KEY.length}
                </div>
                <pre style={{ whiteSpace: "pre-wrap" }}>
                  {JSON.stringify(adyenSession, null, 2)}
                </pre>
              </div>

              {/* ✅ Adyen Drop-in */}
              {adyenSession && (
                <div
                  className={styles.item_buy_content_section}
                  style={{ marginTop: 12 }}
                >
                  <h4>カード決済（Adyen）</h4>
                  <div ref={adyenContainerRef} />
                  <button
                    type="button"
                    onClick={() => {
                      setAdyenSession(null);
                      setProcessing(false);
                    }}
                    style={{ marginTop: 10 }}
                  >
                    戻る
                  </button>
                </div>
              )}

              {payment === "card" && !adyenSession && (
                <div className={styles.oneClickBox}>
                  <div className={styles.oneClickRow}>
                    <div className={styles.oneClickTitle}>
                      One-click（保存カード）
                    </div>

                    {!oneClickUiEnabled ? (
                      <span className={styles.oneClickHint}>
                        Adyen決済中は利用できません
                      </span>
                    ) : walletLoading ? (
                      <span className={styles.oneClickHint}>確認中...</span>
                    ) : oneClickAvailable ? (
                      <label className={styles.oneClickSwitch}>
                        <input
                          type="checkbox"
                          checked={oneClickEnabled}
                          onChange={(e) => setOneClickEnabled(e.target.checked)}
                          disabled={processing || saveCardLoading}
                        />
                        <span>使用する</span>
                      </label>
                    ) : (
                      <span className={styles.oneClickHint}>
                        保存カードなし（または利用不可）
                      </span>
                    )}
                  </div>

                  {oneClickAvailable && defaultPmLabel && (
                    <div className={styles.oneClickCardInfo}>
                      {defaultPmLabel}
                    </div>
                  )}

                  <div className={styles.oneClickNote}>
                    ※ One-click は「画面遷移なしで確定」ですが、必要な場合のみ
                    3DS 認証画面が出ます。
                  </div>
                </div>
              )}
            </div>

            {/* ✅ Stripeカード入力欄：Adyen中は出さない */}
            {needsCardInput && (
              <div className={styles.item_buy_content_section}>
                <h4>カード情報（Stripe）</h4>
                <div className={styles.stripeCardWrapper}>
                  <CardElement
                    options={{
                      hidePostalCode: true,
                      disableLink: true,
                      style: { base: { fontSize: "16px" } },
                    }}
                  />
                </div>

                {payment === "card" && !oneClickAvailable && (
                  <div className={styles.oneClickBox} style={{ marginTop: 12 }}>
                    <div className={styles.oneClickRow}>
                      <div className={styles.oneClickTitle}>
                        保存カードとして登録するしますか？
                      </div>
                      <span className={styles.oneClickHint}>
                        One-click用にカードを保存できます
                      </span>
                    </div>

                    <div
                      style={{
                        marginTop: 10,
                        display: "flex",
                        justifyContent: "flex-start",
                      }}
                    >
                      <button
                        type="button"
                        onClick={saveCardForOneClick}
                        disabled={
                          processing || saveCardLoading || walletLoading
                        }
                        style={{
                          padding: "6px 10px",
                          borderRadius: 8,
                          border: "1px solid #e5e7eb",
                          background: "#fff",
                          fontSize: 13,
                          lineHeight: "18px",
                          cursor: "pointer",
                        }}
                      >
                        {saveCardLoading ? "保存中..." : "このカードを保存"}
                      </button>

                      <span
                        className={styles.oneClickHint}
                        style={{ marginLeft: 10, alignSelf: "center" }}
                      >
                        （One-click用）
                      </span>
                    </div>

                    <div
                      className={styles.oneClickNote}
                      style={{ marginTop: 8 }}
                    >
                      ※ 保存後、自動で「保存カードあり」になれば One-click
                      を使用できます。
                    </div>
                  </div>
                )}
              </div>
            )}

            <div className={styles.item_buy_content_section}>
              <h4>配送先</h4>
              {address ? (
                <div>
                  <p>〒{address.postNumber}</p>
                  <p>
                    {address.prefecture} {address.city}
                  </p>
                  <p>{address.addressLine1}</p>
                </div>
              ) : (
                <p className={styles.warnText}>配送先住所が未登録です</p>
              )}
            </div>
          </div>

          {/* RIGHT */}
          <div className={styles.item_buy_r}>
            <div className={styles.item_buy_summary_box}>
              <p>商品代金: ¥{resolvedItem.price.toLocaleString()}</p>
              <p>
                支払い方法:{" "}
                {payment === "card"
                  ? oneClickEnabled && oneClickAvailable && !adyenSession
                    ? "カード（One-click）"
                    : adyenSession
                      ? "カード（Adyen）"
                      : "カード（入力）"
                  : payment || "未選択"}
              </p>

              <button disabled={!canPurchase} onClick={submitPurchase}>
                {processing
                  ? "処理中..."
                  : oneClickEnabled && oneClickAvailable && payment === "card"
                    ? "ワンクリックで購入"
                    : "購入する"}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
