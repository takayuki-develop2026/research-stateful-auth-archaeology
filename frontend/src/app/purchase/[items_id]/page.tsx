"use client";

import "@adyen/adyen-web/styles/adyen.css";

import React, {
  useMemo,
  useState,
  useEffect,
  useRef,
  forwardRef,
  useImperativeHandle,
} from "react";
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
  {
    locale: "ja",
  },
);

type PaymentMethod = "" | "card" | "konbini";

/**
 * UI上のカードPSPモード
 * - "stripe" | "adyen" | "auto"
 */
type CardPspMode = "stripe" | "adyen" | "auto";
const CARD_PSP_MODE =
  ((process.env.NEXT_PUBLIC_CARD_PSP ?? "auto").trim() as CardPspMode) ||
  "auto";

/* ================= Adyen SDK Loader ================= */
/**
 * ★FIX: Adyenのexport形は環境によって揺れるので、
 * - default を最優先（v6想定）
 * - checkout.create が無い場合は Dropin class を使ってフォールバックできるように
 *   module全体も返す
 */
const loadAdyenSdk = async (): Promise<{
  AdyenCheckout: (config: any) => Promise<any> | any;
  Dropin: any | null;
  Card: any | null;
  mod: any;
}> => {
  const mod: any = await import("@adyen/adyen-web");

  const AdyenCheckout =
    mod?.default ?? mod?.AdyenCheckout ?? mod?.createCheckout ?? mod;

  if (typeof AdyenCheckout !== "function") {
    console.error("[Adyen] module shape:", mod);
    throw new Error("AdyenCheckout function not found");
  }

  // ★強化：Card / Dropin の取り方を増やす（環境差吸収）
  const Dropin =
    mod?.Dropin ?? mod?.default?.Dropin ?? mod?.components?.Dropin ?? null;

  const Card = mod?.Card ?? mod?.default?.Card ?? mod?.components?.Card ?? null;

  return { AdyenCheckout, Dropin, Card, mod };
};

/* ================= Types ================= */

// /orders
type CreateOrderResponse = { order_id: number };

// /payments/start（Stripe/konbini用として残す）
type StartPaymentBase = {
  provider: "stripe" | "adyen";
  payment_id: number;
  status: string;
  provider_payment_id: string | null;
  instructions?: any | null;
};

type StartPaymentResponse =
  | (StartPaymentBase & { provider: "stripe"; client_secret: string | null })
  | (StartPaymentBase & {
      provider: "adyen";
      session_id: string;
      session_data: string;
      client_key?: string | null;
      environment: "test" | "live";
    });

// One-click（Stripe実装のみ）
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

// A案：注文前 preview session
type AdyenPreviewResponse = {
  preview_key: string;
  session_id: string;
  session_data: string;
  environment: "test" | "live";
};

// A案：commit（注文作成＆紐付け）
type AdyenCommitResponse = {
  order_id: number;
  payment_id: number;
};

type AdyenSessionState = {
  orderId: number | null;
  previewKey: string;
  sessionId: string;
  sessionData: string;
  clientKey: string;
  environment: "test" | "live";
};

type ItemShape = any;
type AddressShape = any;

/* =========================================================
   Stripe Section (mounted only when needed)
========================================================= */

type StripeSectionHandle = {
  confirmCardPaymentByClientSecret: (clientSecret: string) => Promise<void>;
  confirmCardSetupByClientSecret: (clientSecret: string) => Promise<void>;
  hasCardElement: () => boolean;
  isCardComplete: () => boolean; // ✅ 追加
};

type StripeSectionProps = {
  processing: boolean;
  saveCardLoading: boolean;
  walletLoading: boolean;
  oneClickAvailable: boolean;
  onSaveCardClick: () => Promise<void>;
};

const StripeCardSection = forwardRef<StripeSectionHandle, StripeSectionProps>(
  function StripeCardSection(props, ref) {
    const stripe = useStripe();
    const elements = useElements();

    // ✅ CardElement の入力完了フラグ（未入力/未完了時の誤実行を防ぐ）
    const [cardComplete, setCardComplete] = useState(false);

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

    useImperativeHandle(
      ref,
      () => ({
        hasCardElement: () => {
          if (!elements) return false;
          return !!elements.getElement(CardElement);
        },

        isCardComplete: () => {
          return !!cardComplete;
        },

        confirmCardPaymentByClientSecret: async (clientSecret: string) => {
          if (!stripe || !elements) {
            throw new Error("Stripe is not ready");
          }
          const card = elements.getElement(CardElement);
          if (!card) {
            throw new Error("CardElement not found");
          }

          const result = await stripe.confirmCardPayment(clientSecret, {
            payment_method: { card },
          });

          if (result.error) {
            throw new Error(
              result.error.message ?? "confirmCardPayment failed",
            );
          }
        },

        confirmCardSetupByClientSecret: async (clientSecret: string) => {
          if (!stripe || !elements) {
            throw new Error("Stripe is not ready");
          }
          const card = elements.getElement(CardElement);
          if (!card) {
            throw new Error("CardElement not found");
          }

          const result = await stripe.confirmCardSetup(clientSecret, {
            payment_method: { card },
          });

          if (result.error) {
            throw new Error(result.error.message ?? "confirmCardSetup failed");
          }

          const status = result.setupIntent?.status;
          if (status !== "succeeded") {
            throw new Error(
              "SetupIntent did not succeed (status != succeeded)",
            );
          }
        },
      }),
      [stripe, elements, cardComplete],
    );

    return (
      <div style={{ marginTop: 12 }}>
        <div className={styles.stripeCardWrapper}>
          <CardElement
            onChange={(e) => {
              // ✅ Stripe Elements 側の入力状態を保持
              setCardComplete(!!(e as any)?.complete);
            }}
            options={{
              hidePostalCode: true,
              disableLink: true,
              style: { base: { fontSize: "16px" } },
            }}
          />
        </div>

        {!props.oneClickAvailable && (
          <div className={styles.oneClickBox} style={{ marginTop: 12 }}>
            <div className={styles.oneClickRow}>
              <div className={styles.oneClickTitle}>
                保存カードとして登録しますか？
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
                onClick={props.onSaveCardClick}
                disabled={
                  props.processing ||
                  props.saveCardLoading ||
                  props.walletLoading ||
                  !cardComplete // ✅ 追加：カード入力が完了するまで押せない（見た目は同じ）
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
                {props.saveCardLoading ? "保存中..." : "このカードを保存"}
              </button>

              <span
                className={styles.oneClickHint}
                style={{ marginLeft: 10, alignSelf: "center" }}
              >
                （One-click用）
              </span>
            </div>

            <div className={styles.oneClickNote} style={{ marginTop: 8 }}>
              ※ 保存後、自動で「保存カードあり」になれば One-click
              を使用できます。
            </div>
          </div>
        )}
      </div>
    );
  },
);

/* =========================================================
   Main Page
========================================================= */

export default function PurchaseConfirmPage() {
  const router = useRouter();
  const params = useParams();
  const { apiClient, isAuthenticated, isLoading: isAuthLoading } = useAuth();

  const DEV = process.env.NODE_ENV !== "production";

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

  const [walletLoading, setWalletLoading] = useState(false);
  const [oneClickAvailable, setOneClickAvailable] = useState(false);
  const [oneClickEnabled, setOneClickEnabled] = useState(false);
  const [defaultPmLabel, setDefaultPmLabel] = useState<string | null>(null);

  const [saveCardLoading, setSaveCardLoading] = useState(false);

  const [adyenSession, setAdyenSession] = useState<AdyenSessionState | null>(
    null,
  );
  const adyenContainerRef = useRef<HTMLDivElement | null>(null);

  const initializedSessionIdRef = useRef<string | null>(null);
  const dropinRef = useRef<any | null>(null);

  // ★FIX: orderId は ref が正（stateに依存しない）
  const orderIdRef = useRef<number | null>(null);

  const stripeSectionRef = useRef<StripeSectionHandle | null>(null);
  const saveCardInFlightRef = useRef(false);
  const [showDebug, setShowDebug] = useState(false);
  const [cardPsp, setCardPsp] = useState<CardPspMode>(CARD_PSP_MODE);

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

  const resolveEffectiveCardPsp = (item: any): "stripe" | "adyen" => {
    if (cardPsp === "stripe") return "stripe";
    if (cardPsp === "adyen") return "adyen";

    const p = item?.shop?.payment_provider ?? item?.shop_payment_provider;
    return p === "adyen" ? "adyen" : "stripe";
  };

  useEffect(() => {
    let cancelled = false;

    const run = async () => {
      setOneClickAvailable(false);
      setDefaultPmLabel(null);
      setOneClickEnabled(false);

      if (!apiClient) return;
      if (!isAuthenticated) return;
      if (payment !== "card") return;

      const effective = resolveEffectiveCardPsp(item);
      if (effective !== "stripe") return;

      try {
        setWalletLoading(true);
        const res = await apiClient.get<WalletPaymentMethodsResponse>(
          "/wallet/payment-methods",
        );
        if (cancelled) return;
        applyWalletState(res);
      } catch (e) {
        if (cancelled) return;
        console.error("[wallet/payment-methods] failed", e);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payment, apiClient, isAuthenticated, cardPsp]);

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

  // A案：カード選択＆Adyen確定時に preview session を作る
  useEffect(() => {
    let cancelled = false;

    const clearAdyen = () => {
      try {
        dropinRef.current?.unmount?.();
      } catch {}
      dropinRef.current = null;
      initializedSessionIdRef.current = null;

      // ★FIX: セッション作り直し時に orderIdRef をクリア
      orderIdRef.current = null;

      setAdyenSession(null);

      if (adyenContainerRef.current) {
        adyenContainerRef.current.innerHTML = "";
      }
    };

    const run = async () => {
      if (!apiClient) return;
      if (!isAuthenticated) {
        clearAdyen();
        return;
      }
      if (payment !== "card") {
        clearAdyen();
        return;
      }
      if (!item) return;

      const effective = resolveEffectiveCardPsp(item);
      if (effective !== "adyen") {
        clearAdyen();
        return;
      }

      if (!ADYEN_CLIENT_KEY) {
        clearAdyen();
        alert("Adyen clientKey が未設定です（NEXT_PUBLIC_ADYEN_CLIENT_KEY）。");
        return;
      }

      if (adyenSession?.sessionId) return;

      try {
        setProcessing(true);

        const amountValue = item?.price != null ? Number(item.price) : 0;
        const res = await apiClient.post<AdyenPreviewResponse>(
          "/payments/adyen/session/preview",
          {
            shop_id: item.shop_id,
            amount: amountValue,
            currency: "JPY",
          },
        );

        if (cancelled) return;

        if (!res?.preview_key || !res?.session_id || !res?.session_data) {
          throw new Error("preview response missing fields");
        }

        setAdyenSession({
          orderId: null,
          previewKey: res.preview_key,
          sessionId: res.session_id,
          sessionData: res.session_data,
          clientKey: ADYEN_CLIENT_KEY,
          environment: res.environment ?? "test",
        });

        // mount側でprocessing解除
      } catch (e) {
        console.error("[AdyenPreview] failed", e);
        if (!cancelled) {
          alert(
            "カード決済（Adyen）の事前セッション生成に失敗しました。\n\n" +
              safeStringifyError(e),
          );
          setProcessing(false);
          clearAdyen();
        }
      }
    };

    run();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payment, apiClient, isAuthenticated, item, cardPsp]);

  // ✅ Adyen Drop-in mount（A案：常時表示 / PayButtonはfalse）
  useEffect(() => {
    if (!adyenSession) return;
    if (!adyenContainerRef.current) return;

    // ここでは "既に同じsessionをマウント済み" のときだけ skip
    if (
      initializedSessionIdRef.current === adyenSession.sessionId &&
      dropinRef.current
    ) {
      return;
    }

    let disposed = false;

    const cleanup = () => {
      disposed = true;
      try {
        dropinRef.current?.unmount?.();
      } catch {}
      dropinRef.current = null;

      // ★超重要：StrictMode 2回目のmountを殺さない
      initializedSessionIdRef.current = null;

      if (adyenContainerRef.current) {
        adyenContainerRef.current.innerHTML = "";
      }
    };

    (async () => {
      try {
        const el = adyenContainerRef.current!;
        el.innerHTML = "";
        el.style.minHeight = "420px";

        const amountValue = item?.price != null ? Number(item.price) : 0;
        const { AdyenCheckout, Dropin, Card, mod } = await loadAdyenSdk();

        const goThanks = (orderId: number) => {
          const url = `/thanks/buy/adyen-card?order_id=${orderId}`;
          window.location.assign(url);
        };

        const handleCompleted = (result: any) => {
          console.log("[Adyen] completed", result);
          console.log("[Adyen] orderIdRef", orderIdRef.current);

          const orderId = orderIdRef.current;
          if (!orderId) {
            alert("決済完了したが orderIdRef が null");
            return;
          }
          goThanks(orderId);
        };

        const handleError = (err: any) => {
          console.error("[Adyen] error", err);
          alert(
            "カード決済（Adyen）でエラーが発生しました。\n\n" +
              safeStringifyError(err),
          );
          setProcessing(false);
        };

        // ✅ コールバックは checkout 側にも載せる（環境差吸収）
        const checkout = await AdyenCheckout({
          environment: adyenSession.environment,
          clientKey: adyenSession.clientKey,
          session: {
            id: adyenSession.sessionId,
            sessionData: adyenSession.sessionData,
          },
          locale: "ja-JP",
          countryCode: "JP",
          amount: { currency: "JPY", value: amountValue },

          onPaymentCompleted: handleCompleted,
          onError: handleError,
        });

        if (disposed) return;

        const CardClass = Card ?? mod?.Card ?? mod?.default?.Card;
        if (!CardClass) throw new Error("Card component class not found");

        const dropinConfig: any = {
          showPayButton: false,
          openFirstPaymentMethod: true,
          paymentMethodComponents: [CardClass],
          paymentMethodsConfiguration: {
            card: {
              showPayButton: false,
              hasHolderName: true,
              holderNameRequired: true,
              hideCVC: false,
            },
          },

          // ✅ Drop-in 側にも載せる（どっちで来てもOK）
          onPaymentCompleted: handleCompleted,
          onError: handleError,
        };

        let dropin: any = null;
        if (checkout && typeof checkout.create === "function") {
          dropin = checkout.create("dropin", dropinConfig);
        } else {
          const DropinClass = Dropin ?? mod?.Dropin ?? mod?.default?.Dropin;
          if (!DropinClass) throw new Error("Dropin class not found");
          dropin = new DropinClass(checkout, dropinConfig);
        }

        dropin.mount(el);
        dropinRef.current = dropin;

        // ★超重要：mount 成功後に “マウント済み” を記録
        initializedSessionIdRef.current = adyenSession.sessionId;

        setProcessing(false);
      } catch (e) {
        console.error("[AdyenDropin] init failed", e);
        alert(
          "カード決済（Adyen）の初期化に失敗しました。\n\n" +
            safeStringifyError(e),
        );
        setProcessing(false);
        cleanup();
        setAdyenSession(null);
      }
    })();

    return cleanup;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adyenSession?.sessionId, item?.price]);


  const saveCardForOneClick = async () => {
    // 0) まず簡易ガード（ここはロック前でOK）
    if (!apiClient) {
      alert("APIクライアントが準備できていません。");
      return;
    }
    if (!isAuthenticated) {
      alert("ログインが必要です。");
      return;
    }
    if (!stripeSectionRef.current?.hasCardElement()) {
      alert("カード入力欄が準備できていません。");
      return;
    }
    if (!stripeSectionRef.current?.isCardComplete()) {
      alert("カード情報を最後まで入力してください。");
      return;
    }

    // ✅ 1) 二重実行防止ロック（ここが追加点）
    if (saveCardInFlightRef.current) return;
    saveCardInFlightRef.current = true;

    try {
      setSaveCardLoading(true);

      // 1) setup-intent 作成
      const si = await apiClient.post<CreateSetupIntentResponse>(
        "/wallet/setup-intent",
        {},
      );
      if (!si?.client_secret || !si?.setup_intent_id) {
        alert("setup_intent の情報が取得できませんでした。");
        return; // ✅ finally でロック解除される
      }

      // 2) Stripe confirm
      await stripeSectionRef.current.confirmCardSetupByClientSecret(
        si.client_secret,
      );

      // 3) SoT確定
      await apiClient.post("/wallet/setup-intent/complete", {
        setup_intent_id: si.setup_intent_id,
        provider: "stripe",
        set_default: true,
      });

      // 4) 最新状態を取り直す
      setWalletLoading(true);
      const wallet = await apiClient.get<WalletPaymentMethodsResponse>(
        "/wallet/payment-methods",
      );
      applyWalletState(wallet);

      alert("カードを保存しました。One-clickが利用可能になります。");
    } catch (e: any) {
      console.error("[saveCardForOneClick] failed", e);
      alert(
        e?.response?.data?.message ?? e?.message ?? "カード保存に失敗しました",
      );
    } finally {
      // ✅ 2) 必ず解除（ここが重要）
      saveCardInFlightRef.current = false;
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

  const resolvedItem: ItemShape = item;
  const resolvedAddress: AddressShape | null = address ?? null;

  const effectiveCardPsp = resolveEffectiveCardPsp(resolvedItem);

  const needsStripeCardInput =
    payment === "card" &&
    effectiveCardPsp === "stripe" &&
    (!oneClickAvailable || !oneClickEnabled);

  const canPurchase =
    isAuthenticated &&
    resolvedItem.remain > 0 &&
    payment !== "" &&
    !!resolvedAddress?.id &&
    !processing;

  /* ================= submit ================= */
  const submitPurchase = async () => {
    if (!canPurchase || !apiClient || !resolvedAddress) return;

    try {
      setProcessing(true);

      // =========================
      // カード（Adyen / A案）
      // =========================
      if (payment === "card" && effectiveCardPsp === "adyen") {
        if (!adyenSession) {
          alert(
            "Adyenセッションがまだ準備できていません。少し待ってから再度お試しください。",
          );
          setProcessing(false);
          return;
        }
        if (!dropinRef.current) {
          alert(
            "Adyen Drop-in が準備できていません。少し待ってから再度お試しください。",
          );
          setProcessing(false);
          return;
        }

        const commit = await apiClient.post<AdyenCommitResponse>(
          "/payments/adyen/commit",
          {
            preview_key: adyenSession.previewKey,
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
            address_id: resolvedAddress.id,
          },
        );

        if (!commit?.order_id || !commit?.payment_id) {
          throw new Error("commit response missing fields");
        }

        // ★FIX: submit前にrefへ確実にセット（stateではなくrefが正）
        orderIdRef.current = commit.order_id;

        setAdyenSession((prev) =>
          prev
            ? {
                ...prev,
                orderId: commit.order_id,
              }
            : prev,
        );

        try {
          dropinRef.current.submit();
        } catch (e) {
          console.error("[Adyen] dropin.submit failed", e);
          alert("Adyen決済の送信に失敗しました。\n\n" + safeStringifyError(e));
          setProcessing(false);
          return;
        }

        // onPaymentCompleted/onError に任せる
        return;
      }

      // =========================
      // それ以外（Stripe/konbini：既存互換）
      // =========================

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
        address_id: resolvedAddress.id,
      });

      // ③ Order 確定
      await apiClient.post(`/orders/${orderId}/confirm`);

      // ④ Payment
      // ---- One-click (Stripe) ----
      if (
        payment === "card" &&
        effectiveCardPsp === "stripe" &&
        oneClickEnabled &&
        oneClickAvailable
      ) {
        const oc = await apiClient.post<OneClickResponse>(
          "/wallet/one-click-checkout",
          {
            order_id: orderId,
          },
        );

        if (oc.requires_action) {
          if (!oc.client_secret) {
            alert("client_secret が取得できませんでした。");
            setProcessing(false);
            return;
          }

          if (!stripeSectionRef.current) {
            alert("Stripe決済UIが準備できていません。");
            setProcessing(false);
            return;
          }

          await stripeSectionRef.current.confirmCardPaymentByClientSecret(
            oc.client_secret,
          );
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

      // konbini
      if (payment !== "card") {
        router.replace(`/thanks/buy/konbini?order_id=${orderId}`);
        return;
      }

      // Stripe
      if (paymentRes.provider === "stripe") {
        if (!paymentRes.client_secret) {
          alert("client_secret が取得できませんでした。");
          setProcessing(false);
          return;
        }

        if (!stripeSectionRef.current) {
          alert("Stripe決済UIが準備できていません。");
          setProcessing(false);
          return;
        }

        await stripeSectionRef.current.confirmCardPaymentByClientSecret(
          paymentRes.client_secret,
        );

        await waitUntilPaid(orderId);
        router.replace(`/thanks/buy/stripe-card?order_id=${orderId}`);
        return;
      }

      alert("想定外のproviderです。設定を確認してください。");
      setProcessing(false);
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
                  ¥{Number(resolvedItem.price).toLocaleString()}
                </p>
              </div>
            </div>

            <div className={styles.item_buy_content_section}>
              <h4>支払い方法</h4>

              <select
                value={payment}
                onChange={(e) => {
                  const v = e.target.value as PaymentMethod;
                  setPayment(v);

                  // カード以外に切り替えたら Adyen セッションは破棄
                  if (v !== "card") {
                    try {
                      dropinRef.current?.unmount?.();
                    } catch {}
                    dropinRef.current = null;
                    initializedSessionIdRef.current = null;

                    // ★FIX: orderIdRef をクリア
                    orderIdRef.current = null;

                    setAdyenSession(null);
                    setProcessing(false);
                    if (adyenContainerRef.current) {
                      adyenContainerRef.current.innerHTML = "";
                    }
                  }
                }}
                disabled={processing}
              >
                <option value="">選択してください</option>
                <option value="konbini">コンビニ支払い</option>
                <option value="card">カード決済</option>
              </select>

              {/* DEV only debug */}
              {DEV && (
                <div style={{ marginTop: 10 }}>
                  <button
                    type="button"
                    onClick={() => setShowDebug((v) => !v)}
                    style={{
                      padding: "6px 10px",
                      borderRadius: 8,
                      border: "1px solid #e5e7eb",
                      background: "#fff",
                      fontSize: 12,
                      cursor: "pointer",
                    }}
                  >
                    {showDebug ? "Debugを隠す" : "Debugを表示(開発用)"}
                  </button>

                  {showDebug && (
                    <div
                      style={{
                        marginTop: 8,
                        padding: 8,
                        border: "1px solid #e5e7eb",
                        background: "#fffbe6",
                        fontSize: 12,
                      }}
                    >
                      <div>debug:processing = {String(processing)}</div>
                      <div>debug:cardPsp(mode) = {cardPsp}</div>
                      <div>debug:effectiveCardPsp = {effectiveCardPsp}</div>
                      <div>
                        debug:ADYEN_CLIENT_KEY(len) = {ADYEN_CLIENT_KEY.length}
                      </div>
                      <div>debug:payment = {payment}</div>
                      <div>debug:orderIdRef = {String(orderIdRef.current)}</div>
                      <pre style={{ whiteSpace: "pre-wrap" }}>
                        {JSON.stringify(adyenSession, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              )}

              {/* ===== カード決済UI（Stripe / Adyen） ===== */}
              {payment === "card" && (
                <div
                  className={styles.item_buy_content_section}
                  style={{ marginTop: 12 }}
                >
                  <h4>
                    カード決済（
                    {effectiveCardPsp === "adyen" ? "Adyen" : "Stripe"}）
                  </h4>

                  {/* One-click（Stripeのみ） */}
                  <div className={styles.oneClickBox}>
                    <div className={styles.oneClickRow}>
                      <div className={styles.oneClickTitle}>
                        One-click（保存カード）
                      </div>

                      {effectiveCardPsp !== "stripe" ? (
                        <span className={styles.oneClickHint}>
                          ※ 現状 One-click は Stripe のみ（Adyen版は未実装）
                        </span>
                      ) : walletLoading ? (
                        <span className={styles.oneClickHint}>確認中...</span>
                      ) : oneClickAvailable ? (
                        <label className={styles.oneClickSwitch}>
                          <input
                            type="checkbox"
                            checked={oneClickEnabled}
                            onChange={(e) =>
                              setOneClickEnabled(e.target.checked)
                            }
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

                    {oneClickAvailable &&
                      defaultPmLabel &&
                      effectiveCardPsp === "stripe" && (
                        <div className={styles.oneClickCardInfo}>
                          {defaultPmLabel}
                        </div>
                      )}

                    <div className={styles.oneClickNote}>
                      ※ One-click は「画面遷移なしで確定」ですが、必要な場合のみ
                      3DS 認証画面が出ます。
                    </div>
                  </div>

                  {/* Adyen Drop-in（A案：カード選択で常時表示） */}
                  {effectiveCardPsp === "adyen" && (
                    <div style={{ marginTop: 12 }}>
                      <div ref={adyenContainerRef} />
                      {!adyenSession && (
                        <div
                          style={{ marginTop: 10, fontSize: 13, opacity: 0.85 }}
                        >
                          Adyenセッションを準備中です...
                        </div>
                      )}
                      {adyenSession && (
                        <div
                          style={{ marginTop: 10, fontSize: 13, opacity: 0.85 }}
                        >
                          ※ カード入力後、「購入する」で確定します
                        </div>
                      )}
                    </div>
                  )}

                  {/* ✅ Stripeは「必要なときだけ」Elementsをマウント */}
                  {effectiveCardPsp === "stripe" && needsStripeCardInput && (
                    <Elements
                      stripe={stripePromise}
                      options={{
                        locale: "ja",
                        appearance: { theme: "stripe" },
                      }}
                    >
                      <StripeCardSection
                        ref={(r) => {
                          stripeSectionRef.current = r;
                        }}
                        processing={processing}
                        saveCardLoading={saveCardLoading}
                        walletLoading={walletLoading}
                        oneClickAvailable={oneClickAvailable}
                        onSaveCardClick={saveCardForOneClick}
                      />
                    </Elements>
                  )}
                </div>
              )}
            </div>

            <div className={styles.item_buy_content_section}>
              <h4>配送先</h4>
              {resolvedAddress ? (
                <div>
                  <p>〒{resolvedAddress.postNumber}</p>
                  <p>
                    {resolvedAddress.prefecture} {resolvedAddress.city}
                  </p>
                  <p>{resolvedAddress.addressLine1}</p>
                </div>
              ) : (
                <p className={styles.warnText}>配送先住所が未登録です</p>
              )}
            </div>
          </div>

          {/* RIGHT */}
          <div className={styles.item_buy_r}>
            <div className={styles.item_buy_summary_box}>
              <p>商品代金: ¥{Number(resolvedItem.price).toLocaleString()}</p>

              <p>
                支払い方法:{" "}
                {payment === "card"
                  ? oneClickEnabled &&
                    oneClickAvailable &&
                    effectiveCardPsp === "stripe"
                    ? "カード（One-click）"
                    : `カード（${effectiveCardPsp === "adyen" ? "Adyen" : "Stripe"}）`
                  : payment || "未選択"}
              </p>

              <button disabled={!canPurchase} onClick={submitPurchase}>
                {processing
                  ? "処理中..."
                  : oneClickEnabled &&
                      oneClickAvailable &&
                      payment === "card" &&
                      effectiveCardPsp === "stripe"
                    ? "ワンクリックで購入"
                    : "購入する"}
              </button>

              {resolvedItem.remain <= 0 && (
                <p style={{ marginTop: 10, color: "#b91c1c" }}>
                  在庫がありません
                </p>
              )}
            </div>

            {/* PSPモード切替（DEVのみ：運用安定後に削除推奨） */}
            {DEV && (
              <div style={{ marginTop: 14, fontSize: 12, opacity: 0.85 }}>
                <div style={{ marginBottom: 6 }}>
                  (開発用): Card PSP
                  Mode(共通ワンクリックカード決済構築中:stripeのみ現在可能)(autoはadminで設定されているPSP(Debugを表示を参照))
                </div>

                <div className={styles.pspModeWrap}>
                  {(["auto", "stripe", "adyen"] as const).map((mode) => {
                    const active = cardPsp === mode;

                    return (
                      <button
                        key={mode}
                        type="button"
                        onClick={() => setCardPsp(mode)}
                        className={`${styles.pspModeBtn} ${
                          active ? styles.pspModeBtnActive : ""
                        }`}
                      >
                        {mode}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
