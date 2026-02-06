/* eslint-disable @next/next/no-img-element */
"use client";

import React, { useState, useRef, useEffect } from "react";
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
     SoT: primary_shop
  ========================= */
  useEffect(() => {
    if (!user) return;

    // 個人用ショップ（必須）
    const personalShop = user.shop_roles?.find(
      (r) => r.role === "owner" // or personal
    );

    if (!personalShop) {
      setError("個人ショップが見つかりません");
      return;
    }

    setSelectedShopId(personalShop.shop_id);
  }, [user]);

  // Auth 初期化待ち
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

    setIsSubmitting(true);
    setError("");

    try {
      // 1. Draft 作成
      type CreateItemDraftResponse = {
        draft_id: string;
      };

      const draftRes = await apiClient.post<CreateItemDraftResponse>(
        "/items/drafts",
        {
          seller_id: `individual:${user.id}`,
          name: form.name,
          price_amount: Number(form.price),
          price_currency: "JPY",
          brand: form.attributes || null,
          explain: form.explain || null,
          category: form.categories.length ? form.categories : null,
        }
      );

      const draftId = draftRes.draft_id;

      // 2. Image Upload
      const imageData = new FormData();
      imageData.append("image", imageFile);

      await apiClient.post(`/items/drafts/${draftId}/image`, imageData);

      // 3. Publish
      await apiClient.post(`/items/drafts/${draftId}/publish`, {
        item_origin: itemOrigin,
        shop_id: selectedShopId,
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
          <label>出品名義</label>
          <div className={styles.radioGroup}>
            <label>
              <input
                type="radio"
                checked={itemOrigin === "USER_PERSONAL"}
                onChange={() => setItemOrigin("USER_PERSONAL")}
              />
              個人出品
            </label>

            <label>
              <input
                type="radio"
                checked={itemOrigin === "SHOP_MANAGED"}
                onChange={() => setItemOrigin("SHOP_MANAGED")}
              />
              ショップ管理商品
            </label>
          </div>
        </div>

        {/* ショップ選択 */}
        {itemOrigin === "SHOP_MANAGED" && user.shop_roles?.length ? (
          <div className={styles.formGroup}>
            <label>出品するショップ</label>
            <select
              value={selectedShopId ?? ""}
              onChange={(e) => setSelectedShopId(Number(e.target.value))}
              required
            >
              <option value="">選択してください</option>
              {user.shop_roles.map((r) => (
                <option key={r.shop_id} value={r.shop_id}>
                  ショップID #{r.shop_id}
                </option>
              ))}
            </select>
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
