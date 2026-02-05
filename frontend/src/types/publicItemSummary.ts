export type PublicItemSummary = {
  id: number;
  name: string;
  price: number | null;
  itemImagePath: string | null;

  remain?: number | null;
  is_sold_out?: boolean | null;

  brandPrimary: string | null;
  conditionName: string | null;
  colorName: string | null;
  publishedAt: string | null;

  displayType: "STAR" | "OWN" | "FAVORITE" | null;

  isOwner: boolean;
  canManage: boolean;
  isFavorited: boolean;
  favoritesCount: number;
};
