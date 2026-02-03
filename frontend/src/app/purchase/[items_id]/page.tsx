"use client";

import React, { useMemo, useState, useEffect } from "react";
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

type CreateOrderResponse = { order_id: number };

// ✅ discriminated union（narrowing できる）
type StartPaymentResponse =
  | { provider: "stripe"; client_secret: string }
  | {
      provider: "adyen";
      session_id: string;
      session_data: string;
      client_key: string;
      environment: "test" | "live";
    };

// ✅ missing エラー対策：型を復活
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

// ✅ Adyen Drop-in 表示用 state（最小）
type AdyenSessionState = {
  orderId: number;
  sessionId: string;
  sessionData: string;
  clientKey: string;
  environment: "test" | "live";
};

/* ================= Page ================= */
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

  // --- Adyen state（start が adyen を返したらここに入る） ---
  const [adyenSession, setAdyenSession] = useState<AdyenSessionState | null>(
    null,
  );

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

  // ✅ Adyen中（Drop-in起動中）はカード入力UIを出さない（後でDrop-inを表示する）
  const isAdyenFlow = adyenSession !== null;

  // ✅ OneClick は最短では Stripe 限定（Adyen時は無効化）
  const oneClickUiEnabled = !isAdyenFlow;

  // ✅ カード入力が必要かどうか（保存カードが無い OR One-clickを使わない）
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
    !isAdyenFlow; // Adyen中は二重送信防止

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
      // ---- One-click (保存カード) ----
      // ✅ 最短：OneClickはStripe限定（Adyen start が返る可能性があるので provider 判定できるまで触らない）
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

        // 3DSなどが必要ならここで実行（Stripe）
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

      // ---- 通常（別カード入力 / 保存カードなし / One-click OFF）----
      const paymentRes = await apiClient.post<StartPaymentResponse>(
        "/payments/start",
        { order_id: orderId, method: payment },
      );

      // card 以外（konbini）: 既存のまま
      if (payment !== "card") {
        router.replace(`/thanks/buy/konbini?order_id=${orderId}`);
        return;
      }

      // ✅ providerで分岐（narrowing）
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

      // ✅ Adyen（Sessions + Drop-in）
      // ここでは state をセットして “Drop-in表示モード” に切替だけ行う
      setAdyenSession({
        orderId,
        sessionId: paymentRes.session_id,
        sessionData: paymentRes.session_data,
        clientKey: paymentRes.client_key,
        environment: paymentRes.environment,
      });

      // processing は Drop-in 側で解除/完了する想定
      // いったんここでは解除しない（ボタン二重押し防止のため）
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

              {/* ✅ Adyen flow中は Drop-in をここに出す（次の段階で実装） */}
              {adyenSession && (
                <div
                  className={styles.item_buy_content_section}
                  style={{ marginTop: 12 }}
                >
                  <h4>カード決済（Adyen）</h4>
                  <div className={styles.oneClickHint}>
                    Adyen決済を開始しました（Drop-in表示は次の実装で追加します）
                  </div>
                  <div className={styles.oneClickHint}>
                    session_id: {adyenSession.sessionId}
                  </div>

                  {/* 取消ボタン（最低限） */}
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
