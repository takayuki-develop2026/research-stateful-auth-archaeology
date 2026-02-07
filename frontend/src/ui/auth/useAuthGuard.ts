"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/ui/auth/AuthProvider";

type GuardOptions = {
  /**
   * Public pages mode:
   * - guest OK
   * - unverified OK
   * - ONLY (logged-in AND verified AND profile-not-completed) -> /mypage/profile
   */
  verifiedProfileOnly?: boolean;

  /** Protected pages mode (default) */
  requireAuth?: boolean;
  requireVerified?: boolean;
  requireProfile?: boolean;

  /** owner dashboard redirect競合回避など */
  allowJustLoggedInBypass?: boolean;

  /** guard対象外にしたいパス（必要なら追加） */
  ignorePaths?: string[]; // e.g. ["/auth/callback", "/login"]
};

function normalizeBool(v: any): boolean {
  if (v === true) return true;
  if (v === false) return false;
  if (v === 1 || v === "1") return true;
  if (v === 0 || v === "0") return false;
  return false;
}

function isIgnored(pathname: string, ignore: string[]) {
  return ignore.some((p) => pathname === p || pathname.startsWith(p));
}

export function useAuthGuard(options: GuardOptions = {}) {
  const {
    verifiedProfileOnly = false,

    requireAuth = true,
    requireVerified = true,
    requireProfile = true,

    allowJustLoggedInBypass = true,
    ignorePaths = ["/auth/callback", "/login"],
  } = options;

  const { user, isAuthenticated, isLoading, authReady } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (isLoading) return;
    if (!authReady) return;

    // 無限ループ防止：コールバックやログイン画面はガードしない
    if (isIgnored(pathname, ignorePaths)) return;

    // =========================================================
    // ✅ Public mode: verifiedProfileOnly
    // - guest OK
    // - unverified OK
    // - ONLY (logged-in AND verified AND profile-not-completed) -> /mypage/profile
    // =========================================================
    if (verifiedProfileOnly) {
      // 未ログインは対象外（公開ページなので何もしない）
      if (!isAuthenticated || !user) return;

      // owner dashboard へ飛ぶ瞬間の競合を避けたい場合だけバイパス
      if (allowJustLoggedInBypass) {
        const justLoggedIn =
          typeof window !== "undefined" &&
          sessionStorage.getItem("occore_just_logged_in_v1") === "1";
        if (justLoggedIn) return;
      }

      // ✅ 未認証は対象外（=公開OK）
      const emailVerified = !!(user as any)?.email_verified_at;
      if (!emailVerified) return;

      // ✅ 認証済み & プロフィール未完了だけ profile へ
      const profileCompleted = normalizeBool((user as any)?.profile_completed);
      if (!profileCompleted && pathname !== "/mypage/profile") {
        router.replace("/mypage/profile");
      }
      return;
    }

    // =========================================================
    // ✅ Protected mode (default)
    // =========================================================

    // 1) 未ログイン → /login
    if (requireAuth && (!isAuthenticated || !user)) {
      router.replace("/login");
      return;
    }
    if (!isAuthenticated || !user) return;

    // owner dashboard 直後だけ bypass したい時用（必要なら条件追加）
    if (allowJustLoggedInBypass) {
      const justLoggedIn =
        typeof window !== "undefined" &&
        sessionStorage.getItem("occore_just_logged_in_v1") === "1";

      const isOwner =
        Array.isArray((user as any)?.shop_roles) &&
        (user as any).shop_roles.some((r: any) => r?.role === "owner");

      if (justLoggedIn && isOwner && pathname.startsWith("/shops/")) return;
    }

    // 2) 未認証 → /email/verify
    const emailVerified = !!(user as any)?.email_verified_at;
    if (requireVerified && !emailVerified) {
      if (pathname !== "/email/verify") router.replace("/email/verify");
      return;
    }

    // 3) profile未完了 → /mypage/profile
    const profileCompleted = normalizeBool((user as any)?.profile_completed);
    if (requireProfile && !profileCompleted) {
      if (pathname !== "/mypage/profile") router.replace("/mypage/profile");
      return;
    }
  }, [
    isLoading,
    authReady,
    isAuthenticated,
    user,
    router,
    pathname,
    verifiedProfileOnly,
    requireAuth,
    requireVerified,
    requireProfile,
    allowJustLoggedInBypass,
    ignorePaths,
  ]);
}
