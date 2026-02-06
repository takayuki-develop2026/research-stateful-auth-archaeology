"use client";

import React, { useMemo } from "react";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";

import { useItemListByShopSWR } from "@/services/useItemListByShopSWR";
import { useItemSearchByShopSWR } from "@/services/useItemSearchByShopSWR";
import { useAuth } from "@/ui/auth/AuthProvider";
import { useShopAccess } from "@/ui/shop/useShopAccess";

import type { Item } from "@/types/item";
import { getImageUrl } from "@/utils/utils";
import styles from "./W-Shop-Home.module.css";

export default function ShopHomePage() {
  const { shop_code } = useParams<{ shop_code: string }>();
  const searchParams = useSearchParams();

  const { isAuthenticated, authReady, isLoading: isAuthLoading } = useAuth();

  const shopAccess = useShopAccess(shop_code);

  const currentSearchQuery = useMemo(
    () => searchParams.get("q") || "",
    [searchParams],
  );

  const isSearch = currentSearchQuery.trim().length > 0;

  const listResult = useItemListByShopSWR(shop_code);
  const searchResult = useItemSearchByShopSWR(shop_code, currentSearchQuery);

  const items: Item[] = isSearch ? searchResult.items : listResult.items;

  const isItemsLoading = isSearch
    ? searchResult.isLoading
    : listResult.isLoading;

  const isPageLoading = !authReady || isAuthLoading || isItemsLoading;

  if (isPageLoading) {
    return <div className={styles.loadingBox}>読み込み中...</div>;
  }

  return (
    <div className={styles.main_contents}>
      <div className={styles.shopHeader}>
        <h1 className={styles.title}>Shop: {shop_code}</h1>

        {shopAccess.canAccessDashboard && (
          <Link
            href={`/shops/${shop_code}/dashboard`}
            className={styles.dashboardButton}
          >
            管理画面
          </Link>
        )}
      </div>

      <div className={styles.items_select}>
        {items.length > 0 ? (
          items.map((item) => {
            // ✅ 売り切れ判定（remain==0）
            const isSoldOut = Number((item as any).remain ?? 0) === 0;

            const CardInner = (
              <>
                <div
                  className={`${styles.imageWrap} ${
                    isSoldOut ? styles.imageWrapSoldOut : ""
                  }`}
                >
                  <img src={getImageUrl(item.item_image)} alt={item.name} />

                  {isSoldOut && (
                    <div className={styles.soldOutBadge}>完売</div>
                  )}
                </div>

                <div className={styles.cardBody}>
                  <p className={styles.itemName}>{item.name}</p>
                  <p className={styles.itemPrice}>
                    ¥
                    {typeof item.price === "number"
                      ? item.price.toLocaleString()
                      : "-"}
                  </p>
                </div>
              </>
            );

            return (
              <div key={item.id} className={styles.items_select_all}>
                {isSoldOut ? (
                  // ✅ 売り切れはクリック不可（表示だけAの最小安全運用）
                  <div
                    className={`${styles.cardLink} ${styles.cardLinkDisabled}`}
                    aria-disabled="true"
                    title="売り切れ"
                  >
                    {CardInner}
                  </div>
                ) : (
                  <Link href={`/item/${item.id}`} className={styles.cardLink}>
                    {CardInner}
                  </Link>
                )}
              </div>
            );
          })
        ) : (
          <div className={styles.no_items}>商品がありません。</div>
        )}
      </div>

      {!isAuthenticated && (
        <div className={styles.notice}>
          ログインすると購入やマイリストが使えます。
        </div>
      )}
    </div>
  );
}
