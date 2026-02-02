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

type CreateOrderResponse = {
  order_id: number;
};

type StartPaymentResponse = {
  client_secret: string;
};

type OneClickResponse = {
  payment_id: number;
  status: string;
  provider_payment_id: string;
  client_secret: string;
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

  // 支払い方法が card の時だけ Wallet を確認
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
        console.log("[🔥wallet/payment-methods]", res); // ★追加
        if (cancelled) return;

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
            `${def.brand.toUpperCase()} **** ${def.last4} (exp ${def.exp_month}/${def.exp_year})`,
          );
          // デフォルトでONにしたいなら true にする。安全寄りなら false のまま。
          setOneClickEnabled(true);
        } else {
          setOneClickEnabled(false);
        }
      } catch {
        if (cancelled) return;
        setOneClickAvailable(false);
        setOneClickEnabled(false);
      } finally {
        if (!cancelled) setWalletLoading(false);
      }
    };

    run();
    return () => {
      cancelled = true;
    };
  }, [payment, apiClient, isAuthenticated]);

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

  const canPurchase =
    isAuthenticated &&
    resolvedItem.remain > 0 &&
    payment !== "" &&
    !!address?.id &&
    !processing &&
    // card の場合：one-click OFF なら CardElement を要求（submit内でも最終チェック）
    (payment !== "card" || oneClickEnabled || true);

  // Order が paid になるのを待つ（Webhookタイミング差吸収）
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
      if (payment === "card" && oneClickEnabled && oneClickAvailable) {
        // ---- One-click ----
        const oc = await apiClient.post<OneClickResponse>(
          "/wallet/one-click-checkout",
          {
            order_id: orderId,
          },
        );

        // 3DSなどが必要ならここで実行
        if (oc.requires_action) {
          if (!stripe) {
            alert("決済の準備が整っていません。");
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

        // webhook遅延に備えて paid を待つ
        await waitUntilPaid(orderId);

        router.replace(`/thanks/buy/stripe-card?order_id=${orderId}`);
        return;
      }

      // ---- 通常（今まで通り）----
      const paymentRes = await apiClient.post<StartPaymentResponse>(
        "/payments/start",
        {
          order_id: orderId,
          method: payment,
        },
      );

      if (payment === "card") {
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
      } else {
        router.replace(`/thanks/buy/konbini?order_id=${orderId}`);
      }
    } catch (e: any) {
      console.error("[🔥wallet/payment-methods] failed", e); // ★追加
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
              >
                <option value="">選択してください</option>
                <option value="konbini">コンビニ支払い</option>
                <option value="card">クレジットカード支払い</option>
              </select>

              {/* One-click toggle (card only) */}
              {payment === "card" && (
                <div className={styles.oneClickBox}>
                  <div className={styles.oneClickRow}>
                    <div className={styles.oneClickTitle}>
                      One-click（保存カード）
                    </div>
                    {walletLoading ? (
                      <span className={styles.oneClickHint}>確認中...</span>
                    ) : oneClickAvailable ? (
                      <label className={styles.oneClickSwitch}>
                        <input
                          type="checkbox"
                          checked={oneClickEnabled}
                          onChange={(e) => setOneClickEnabled(e.target.checked)}
                          disabled={processing}
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

            {/* 通常カード入力：oneClickEnabled がOFFのときだけ */}
            {payment === "card" && !oneClickEnabled && (
              <div className={styles.item_buy_content_section}>
                <h4>カード情報</h4>
                <div className={styles.stripeCardWrapper}>
                  <CardElement
                    options={{
                      hidePostalCode: true,
                      style: { base: { fontSize: "16px" } },
                    }}
                  />
                </div>
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
                  ? oneClickEnabled
                    ? "カード（One-click）"
                    : "カード（入力）"
                  : payment || "未選択"}
              </p>
              <button disabled={!canPurchase} onClick={submitPurchase}>
                {processing
                  ? "処理中..."
                  : oneClickEnabled && payment === "card"
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
