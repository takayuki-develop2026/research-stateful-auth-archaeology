import useSWR from "swr";
import { useAuth } from "@/ui/auth/AuthProvider";
import { useAuthedFetcher } from "@/ui/auth/useAuthedFetcher";
import type { PublicItemSummary } from "@/types/publicItemSummary";

type Response = {
  items: PublicItemSummary[];
};

export const FAVORITE_ITEMS_SWR_KEY = "/favorites"; // ✅ v3 fixed

export const useFavoriteItemsSWR = () => {
  const { authReady, isAuthenticated } = useAuth();
  const fetcher = useAuthedFetcher();

  const swrKey = authReady && isAuthenticated ? FAVORITE_ITEMS_SWR_KEY : null;

  const { data, error, mutate } = useSWR<Response>(
    swrKey,
    () => fetcher.get(FAVORITE_ITEMS_SWR_KEY),
    {
      revalidateOnFocus: false,
      revalidateOnReconnect: false,
      revalidateIfStale: false,
    },
  );

  return {
    items: data?.items ?? [],
    isLoading: !authReady,
    error,
    refetchFavorites: () => mutate(),
  };
};
