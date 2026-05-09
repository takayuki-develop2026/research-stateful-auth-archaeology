"use client";

import React, { useState, useRef, useEffect, useMemo } from "react";
import { useRouter } from "next/navigation";

import { useAuth } from "@/ui/auth/AuthProvider";
import styles from "./W-Item-Sell.module.css";

/* =========================
   Types
========================= */
type SellForm = {
  name: string;
  price: string;
  explain: string;
  attributes: string;
  categories: string[];
};

type ItemOrigin = "USER_PERSONAL" | "SHOP_MANAGED";

/* =========================
   Constants
========================= */
const CATEGORY_LIST = [
  "ファッション",
  "家電",
  "インテリア",
  "レディース",
  "メンズ",
  "コスメ",
  "本",
  "ゲーム",
  "スポーツ",
  "キッチン",
  "ハンドメイド",
  "アクセサリー",
];

/* =========================
   Page
========================= */
export default function ItemSellPage() {
  const router = useRouter();

  const { authReady, isAuthenticated, apiClient, user } = useAuth();

  const fileInputRef = useRef<HTMLInputElement | null>(null);

  /* =========================
     State
  ========================= */
  const [form, setForm] = useState<SellForm>({
    name: "",
    price: "",
    explain: "",
    attributes: "",
    categories: [],
  });

  const [itemOrigin, setItemOrigin] = useState<ItemOrigin>("USER_PERSONAL");
  const [selectedShopId, setSelectedShopId] = useState<number | null>(null);
  const [imageFile, setImageFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");

  /* =========================
     Shop helpers（userがnullでも安全）
  ========================= */
  const shops = user?.shop_roles ?? [];

  // ✅ 一般ユーザーは SHOP_MANAGED を選べない（owner/manager のみ）
  const canShopManaged = useMemo(() => {
    return shops.some((r: any) => r.role === "owner" || r.role === "manager");
  }, [shops]);

  // ✅ 表示用ラベル（shop_name 等があれば使う）
  const shopLabel = (r: any) => {
    return (
      r.shop_name ??
      r.shopName ??
      r.shop_code ??
      r.shopCode ??
      `ショップID #${r.shop_id}`
    );
  };

  const selectedShop = useMemo(() => {
    return shops.find((r: any) => r.shop_id === selectedShopId) ?? null;
  }, [shops, selectedShopId]);

  /* =========================
     Auth Guard（共通仕様）
  ========================= */
  useEffect(() => {
    if (!authReady) return;

    if (!isAuthenticated) {
      router.replace("/login");
    }
  }, [authReady, isAuthenticated, router]);

  /* =========================
     SHOP_MANAGED 初期化
     - USER_PERSONAL のときは shop を要求しない（エラー出さない）
     - SHOP_MANAGED のときだけ自動選択（owner優先）
  ========================= */
  useEffect(() => {
    if (!user) return;

    // ✅ USER_PERSONAL では shop を要求しない
    if (itemOrigin !== "SHOP_MANAGED") {
      setError("");
      return;
    }

    // ✅ SHOP_MANAGED を選べないユーザーは UIもdisabledだが二重防御
    if (!canShopManaged) {
      setError("");
      setItemOrigin("USER_PERSONAL");
      return;
    }

    const ownerShop = shops.find((r: any) => r.role === "owner") ?? null;
    const fallbackShop =
      ownerShop ?? shops.find((r: any) => r.role === "manager") ?? null;

    if (!fallbackShop) {
      setError("ショップ出品する権限がありません");
      return;
    }

    setSelectedShopId(fallbackShop.shop_id);
    setError("");
  }, [user, itemOrigin, canShopManaged, shops]);

  /* =========================
     ここで早期return（Hooksの後なのでOK）
  ========================= */
  if (!authReady || !user) return null;

  /* =========================
     Image Select
  ========================= */
  const handleImageChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setImageFile(file);
    setPreviewUrl(URL.createObjectURL(file));
  };

  /* =========================
     Category Toggle
  ========================= */
  const toggleCategory = (category: string) => {
    setForm((prev) => ({
      ...prev,
      categories: prev.categories.includes(category)
        ? prev.categories.filter((c) => c !== category)
        : [...prev.categories, category],
    }));
  };

  /* =========================
     Submit（DDD 正式フロー）
  ========================= */
  const submitItem = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!apiClient || !user) {
      setError("ログイン状態が確認できません");
      return;
    }

    if (!imageFile) {
      setError("画像を選択してください");
      return;
    }

    // ✅ SHOP_MANAGED のときだけ shop 選択必須
    if (itemOrigin === "SHOP_MANAGED") {
      if (!canShopManaged) {
        setError("ショップ出品する権限がありません");
        return;
      }
      if (!selectedShopId) {
        setError("ショップを選択してください");
        return;
      }
    }

    setIsSubmitting(true);
    setError("");

    try {
      // 1. Draft 作成
      type CreateItemDraftResponse = { draft_id: string };

      const sellerId =
        itemOrigin === "SHOP_MANAGED"
          ? `shop:${selectedShopId}`
          : `individual:${user.id}`;

      const draftRes = await apiClient.post<CreateItemDraftResponse>(
        "/items/drafts",
        {
          seller_id: sellerId,
          name: form.name,
          price_amount: Number(form.price),
          price_currency: "JPY",
          brand: form.attributes || null,
          explain: form.explain || null,
          category: form.categories.length ? form.categories : null,
        },
      );

      const draftId = draftRes.draft_id;

      // 2. Image Upload
      const imageData = new FormData();
      imageData.append("image", imageFile);
      await apiClient.post(`/items/drafts/${draftId}/image`, imageData);

      // 3. Publish（SHOPのときだけ shop_id 送信）
      await apiClient.post(`/items/drafts/${draftId}/publish`, {
        item_origin: itemOrigin,
        ...(itemOrigin === "SHOP_MANAGED" ? { shop_id: selectedShopId } : {}),
      });

      router.push("/");
    } catch (e) {
      console.error(e);
      setError("商品の出品に失敗しました");
    } finally {
      setIsSubmitting(false);
    }
  };

  /* =========================
     UI（初期デザイン完全保持）
  ========================= */
  return (
    <div className={styles.wrapper}>
      <h2 className={`${styles.title} ${styles.centerTitle}`}>商品の出品</h2>

      <form onSubmit={submitItem} className={styles.form}>
        {/* 出品名義 */}
        <div className={styles.formGroup}>
          <label>出品タイプ</label>
          <div className={styles.radioGroup}>
            <label>
              <input
                type="radio"
                checked={itemOrigin === "USER_PERSONAL"}
                onChange={() => setItemOrigin("USER_PERSONAL")}
              />
              💫カスタマー出品/💫ショップユーザーが個人出品　　
            </label>

            {/* ✅ 一般ユーザーは薄い灰色の◯で選択不可 */}
            <label
              style={{
                opacity: canShopManaged ? 1 : 0.45,
                cursor: canShopManaged ? "pointer" : "not-allowed",
              }}
              title={
                canShopManaged
                  ? ""
                  : "ショップ出品する権限がありません（owner/manager が必要）"
              }
            >
              <input
                type="radio"
                checked={itemOrigin === "SHOP_MANAGED"}
                onChange={() => setItemOrigin("SHOP_MANAGED")}
                disabled={!canShopManaged}
              />
              ⭐️ショップ出品（ショップ管理）(解析(現段階では、ブランド・状態・色)はどちらも機能しますが管理できるのはこちら）)
            </label>
          </div>
        </div>

        {/* ショップ選択 */}
        {itemOrigin === "SHOP_MANAGED" && canShopManaged && shops.length ? (
          <div className={styles.formGroup}>
            <label>出品するショップ名</label>
            <select
              value={selectedShopId ?? ""}
              onChange={(e) => setSelectedShopId(Number(e.target.value))}
              required
            >
              <option value="">選択してください</option>
              {shops.map((r: any) => (
                <option key={r.shop_id} value={r.shop_id}>
                  {shopLabel(r)}
                </option>
              ))}
            </select>

            {selectedShop ? (
              <div style={{ marginTop: 8, fontSize: 12, color: "#666" }}>
                選択中：{shopLabel(selectedShop)}
              </div>
            ) : null}
          </div>
        ) : null}

        {/* 画像 */}
        <div className={styles.imageBoxWide}>
          <div className={styles.imageInner}>
            {previewUrl && <img src={previewUrl} className={styles.preview} />}

            <button
              type="button"
              className={styles.imageButton}
              onClick={() => fileInputRef.current?.click()}
            >
              画像を選択する
            </button>
          </div>

          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            hidden
            onChange={handleImageChange}
          />
        </div>

        {/* カテゴリー */}
        <div className={styles.formGroup}>
          <label>カテゴリー（複数選択）</label>
          <div className={styles.categoryButtons}>
            {CATEGORY_LIST.map((cat) => (
              <button
                key={cat}
                type="button"
                className={
                  form.categories.includes(cat)
                    ? styles.categoryActive
                    : styles.categoryButton
                }
                onClick={() => toggleCategory(cat)}
              >
                {cat}
              </button>
            ))}
          </div>
        </div>

        {/* brand / condition / color */}
        <div className={styles.formGroup}>
          <label>
            ブランド・状態・色、順不同でも機能する。（まとめて入力可能でどのような複雑なデーターでも処理できる開発をしています。）
          </label>
          <input
            type="text"
            placeholder="例：Appleほぼ新品黒など(スペース,コンマなど有無でも可能+ひらがな,カタカナ,英語混合可能)"
            value={form.attributes}
            onChange={(e) =>
              setForm((v) => ({
                ...v,
                attributes: e.target.value,
              }))
            }
          />
          <small className={styles.hint}>
            ※ 入力内容は自動で解析・正規化されます
            ※企業判断や成長企画や実績に未来再利用可能な形で蓄積するエンジン開発のプロトタイプです。
          </small>
        </div>

        {/* 商品名 */}
        <div className={styles.formGroup}>
          <label>商品名</label>
          <input
            type="text"
            value={form.name}
            onChange={(e) =>
              setForm((v) => ({
                ...v,
                name: e.target.value,
              }))
            }
            required
          />
        </div>

        {/* 商品説明 */}
        <div className={styles.formGroup}>
          <label>商品説明</label>
          <textarea
            rows={6}
            value={form.explain}
            onChange={(e) =>
              setForm((v) => ({
                ...v,
                explain: e.target.value,
              }))
            }
          />
        </div>

        {/* 価格 */}
        <div className={styles.formGroup}>
          <label>価格</label>
          <input
            type="number"
            placeholder="¥"
            value={form.price}
            onChange={(e) =>
              setForm((v) => ({
                ...v,
                price: e.target.value,
              }))
            }
            required
          />
        </div>

        {error && <p className={styles.error}>{error}</p>}

        <div className={styles.actions}>
          <button type="submit" disabled={isSubmitting}>
            出品する
          </button>
        </div>
      </form>
    </div>
  );
}
