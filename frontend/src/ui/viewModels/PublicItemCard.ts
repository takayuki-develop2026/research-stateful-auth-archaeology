export type PublicItemCard = {
  id: number;
  name: string;
  price: number | null;
  itemImagePath: string | null;
  displayType: "STAR" | "OWN" | "FAVORITE" | null;
  isFavorited: boolean;
  remain: number | null;
  isSoldOut: boolean;
};
