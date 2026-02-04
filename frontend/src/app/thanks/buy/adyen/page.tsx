"use client";

import { useSearchParams } from "next/navigation";

export default function ThanksBuyAdyenPage() {
  const sp = useSearchParams();
  const orderId = sp.get("order_id");
  const paid = sp.get("paid");

  return (
    <div style={{ padding: 24 }}>
      <h1>購入ありがとうございました（Adyen）</h1>
      <p>order_id: {orderId}</p>
      <p>paid: {paid}</p>
      {paid === "1" ? (
        <p>決済完了を確認しました。</p>
      ) : (
        <p>決済確認中です。注文詳細を確認してください。</p>
      )}
    </div>
  );
}
