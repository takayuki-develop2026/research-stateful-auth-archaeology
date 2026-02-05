"use client";

import { useEffect, useMemo } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { mutate } from "swr";

import { useAuthGuard } from "@/ui/auth/useAuthGuard";
import { useAuth } from "@/ui/auth/AuthProvider";

import { useItemListSWR } from "@/services/useItemListSWR";
import { useItemSearchSWR } from "@/services/useItemSearchSWR";
import { useFavoriteItemsSWR } from "@/services/useFavoriteItemsSWR";

import type { PublicItemCard } from "@/ui/viewModels/PublicItemCard";
import { getImageUrl, IMAGE_TYPE, onImageError } from "@/utils/utils";

import styles from "./W-Resource-Rich-Simulation-Center-Home.module.css";

export default function Home() {
  useAuthGuard();

  const router = useRouter();
  const searchParams = useSearchParams();
  const { isAuthenticated, authReady, apiClient, user } = useAuth();

  useEffect(() => {
    if (!authReady || !isAuthenticated) return;

    const just = sessionStorage.getItem("occore_just_logged_in_v1");
    if (just !== "1") return;

    const shopCode = sessionStorage.getItem("occore_owner_shop_code_v1");

    const r0 = (user as any)?.shop_roles?.[0];
    const fallback = r0?.role === "owner" ? r0?.shop_code : null;

    const code = shopCode || fallback;
    if (!code) return;

    router.replace(`/shops/${code}/dashboard`);

    setTimeout(() => {
      sessionStorage.removeItem("occore_just_logged_in_v1");
      sessionStorage.removeItem("occore_owner_shop_code_v1");
    }, 1500);
  }, [authReady, isAuthenticated, user, router]);

  /* =========================
     Tab / Search
  ========================= */
  const currentTab = useMemo<"all" | "mylist">(
    () => (searchParams.get("tab") === "mylist" ? "mylist" : "all"),
    [searchParams],
  );

  const searchQuery = useMemo(
    () => searchParams.get("all_item_search") ?? "",
    [searchParams],
  );

  const isSearch = searchQuery.trim().length > 0;

  /* =========================
     Hooks（常に呼ぶ）
  ========================= */
  const listResult = useItemListSWR();
  const searchResult = useItemSearchSWR(searchQuery);
  const favoriteResult = useFavoriteItemsSWR();

  const isItemsLoading =
    listResult.isLoading || searchResult.isLoading || favoriteResult.isLoading;

  /* =========================
     Normalize → ViewModel
  ========================= */
  const items: PublicItemCard[] = useMemo(() => {
    const raw =
      currentTab === "mylist"
        ? favoriteResult.items.map((item) => ({ ...item, displayType: null }))
        : isSearch
          ? searchResult.items
          : listResult.items;

    return raw.map((item: any) => {
      // price: number or {amount}
      const price =
        typeof item.price === "object"
          ? item.price?.amount
          : (item.price ?? null);

      const itemImagePath = item.item_image_path ?? item.itemImagePath ?? null;

      // ✅ remain / is_sold_out（両対応）
      const remain =
        typeof item.remain === "number"
          ? item.remain
          : typeof item.stock_remain === "number"
            ? item.stock_remain
            : null;

      const isSoldOut =
        typeof item.is_sold_out === "boolean"
          ? item.is_sold_out
          : remain !== null
            ? remain <= 0
            : false;

      return {
        id: item.id,
        name: item.name,
        price,
        itemImagePath,
        displayType: item.displayType ?? null,
        isFavorited: Boolean(item.isFavorited),

        // ✅ 追加
        remain,
        isSoldOut,
      };
    });
  }, [
    currentTab,
    isSearch,
    listResult.items,
    searchResult.items,
    favoriteResult.items,
  ]);

  const isPageLoading = !authReady || isItemsLoading;

  /* =========================
     Favorite toggle
  ========================= */
  const toggleFavorite = async (item: PublicItemCard, isFavorited: boolean) => {
    if (!apiClient) return;

    try {
      if (isFavorited) {
        await apiClient.delete(`/favorites/${item.id}`);
      } else {
        await apiClient.post(`/favorites/${item.id}`);
      }
      mutate(() => true);
    } catch (e) {
      console.error(e);
    }
  };

  /* =========================
     Render
  ========================= */
  return (
    <div className={styles.main_contents}>
      {isPageLoading && (
        <div className={styles.loadingBox}>
          <div className={styles.spinner} />
          <p className={styles.loadingText}>読み込み中...</p>
        </div>
      )}

      {!isPageLoading && (
        <>
          {/* 🏪 ショップ別ホームリンク */}
          <div className={styles.shopButtons}>
            {["a", "b", "c", "d"].map((code) => (
              <button
                key={code}
                className={styles.shopButton}
                onClick={() => router.push(`/shops/shop-${code}`)}
              >
                テスト ショップ {code.toUpperCase()} へ
              </button>
            ))}
          </div>

          {/* Tabs */}
          <div className={styles.main_select}>
            <Link
              href={{ pathname: "/", query: { all_item_search: searchQuery } }}
              className={`${styles.recs} ${currentTab === "all" ? styles.active : ""}`}
            >
              すべて
            </Link>

            <Link
              href={{ pathname: "/", query: { tab: "mylist" } }}
              className={`${styles.mylists} ${currentTab === "mylist" ? styles.active : ""}`}
            >
              マイリスト
            </Link>
          </div>

          {/* Items */}
          <div className={styles.items_select}>
            {items.length > 0 ? (
              items.map((item) => {
                const isFavorited = item.isFavorited;

                return (
                  <div key={item.id} className={styles.items_select_all}>
                    {/* ✅ 完売でも詳細は見せる（クリック挙動は変更しない） */}
                    <div
                      className={styles.cardLink}
                      role="button"
                      tabIndex={0}
                      onClick={() => router.push(`/item/${item.id}`)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") router.push(`/item/${item.id}`);
                      }}
                    >
                      <div className={styles.itemImageWrapper}>
                        {item.displayType && (
                          <span className={styles.ownStar}>
                            {item.displayType === "STAR" ? "⭐️" : "💫"}
                          </span>
                        )}

                        {/* ✅ 完売バッジ */}
                        {item.isSoldOut && (
                          <span className={styles.soldOutBadge}>完売</span>
                        )}

                        {isAuthenticated && (
                          <button
                            className={styles.favoriteButton}
                            onClick={(e) => {
                              e.stopPropagation();
                              toggleFavorite(item, isFavorited);
                            }}
                          >
                            {isFavorited ? "❤️" : "🤍"}
                          </button>
                        )}

                        <img
                          src={getImageUrl(item.itemImagePath, IMAGE_TYPE.ITEM)}
                          alt={item.name}
                          className={`${styles.itemImage} ${
                            item.isSoldOut ? styles.itemImageSoldOut : ""
                          }`}
                          onError={onImageError}
                        />
                      </div>

                      <div className={styles.item_info}>
                        <p className={styles.item_name}>{item.name}</p>
                        <p className={styles.item_price}>
                          ¥
                          {typeof item.price === "number"
                            ? item.price.toLocaleString()
                            : "-"}
                        </p>

                        {/* 任意：残数を出したい場合 */}
                        {/* {typeof item.remain === "number" && (
                          <p className={styles.item_remain}>
                            残り {item.remain}
                          </p>
                        )} */}
                      </div>
                    </div>
                  </div>
                );
              })
            ) : (
              <div className={styles.no_items}>
                {currentTab === "mylist" && authReady && !isAuthenticated
                  ? "マイリストを見るにはログインが必要です。"
                  : "該当する商品が見つかりませんでした。"}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
