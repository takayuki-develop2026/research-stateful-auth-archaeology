"use client";

import React, {
  useState,
  useEffect,
  useCallback,
  useRef,
  useMemo,
} from "react";
import { useRouter } from "next/navigation";

import { useAuth } from "@/ui/auth/AuthProvider";
import { getImageUrl, IMAGE_TYPE } from "@/utils/utils";
import styles from "./W-ProfilePage.module.css";
import { useAuthGuard } from "@/ui/auth/useAuthGuard";

/* =========================
   Types
========================= */
interface ProfileUser {
  id: number;
  display_name: string | null;
  email: string;
  post_number: string | null;
  address: string | null;
  building: string | null;
  user_image: string | null;
}

interface ProfileForm {
  display_name: string;
  post_number: string;
  address: string;
  building: string;
}

type ProfileErrors = {
  [K in keyof ProfileForm]?: string[];
} & {
  user_image?: string[];
};



/* =========================
   Component
========================= */
export default function ProfilePage() {
  useAuthGuard({
    requireAuth: true,
    requireVerified: false,
    requireProfile: false, // ✅ 自分自身には “profile未完了→profileへ” をかけない
  });
  const router = useRouter();

  const {
    user: authUser,
    apiClient,
    authReady,
    isAuthenticated,
    isLoading: isAuthLoading,
    logout,
    refresh,
  } = useAuth();

  const [profileUser, setProfileUser] = useState<ProfileUser | null>(null);
  const [hasFetchedProfile, setHasFetchedProfile] = useState(false);

  const [form, setForm] = useState<ProfileForm>({
    display_name: "",
    post_number: "",
    address: "",
    building: "",
  });

  const [profileErrors, setProfileErrors] = useState<ProfileErrors>({});
  const [imageError, setImageError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");

  // 🔹 役割分離
  const [pageLoading, setPageLoading] = useState(true);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [isFetching, setIsFetching] = useState(false);

  const fileInputRef = useRef<HTMLInputElement | null>(null);

  /* =========================
     Image URL
  ========================= */
  const profileImageUrl = useMemo(() => {
    return getImageUrl(profileUser?.user_image ?? null, IMAGE_TYPE.USER);
  }, [profileUser?.user_image]);

  /* =========================
     Helpers
  ========================= */
  const initializeProfile = useCallback((user: ProfileUser | null) => {
    setProfileUser(user);
    setForm({
      display_name: user?.display_name ?? "",
      post_number: user?.post_number ?? "",
      address: user?.address ?? "",
      building: user?.building ?? "",
    });
  }, []);

  /* =========================
     Fetch Profile（1回だけ）
  ========================= */
  const fetchUserProfile = useCallback(async () => {
    if (!apiClient) return;

    setIsFetching(true);
    setProfileErrors({});
    setSuccessMessage("");

    try {
      const res: any = await apiClient.get("/mypage/profile");
      initializeProfile(res?.user ?? res?.data?.user ?? null);
      setHasFetchedProfile(true);
    } catch (err: any) {
      if (err?.message === "Unauthenticated") {
        await logout();
        router.replace("/login");
      }
    } finally {
      setPageLoading(false);
      setIsFetching(false);
    }
  }, [apiClient, initializeProfile, logout, router]);

  /* =========================
     Image Upload
  ========================= */
  const handleImageUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !apiClient) return;

    setImageError("");
    setSubmitLoading(true);

    const formData = new FormData();
    formData.append("user_image", file);

    try {
      const res: any = await apiClient.post("/mypage/profile/image", formData);
      initializeProfile(res?.user ?? res?.data?.user ?? null);
      setSuccessMessage("画像を更新しました！");
    } catch (err: any) {
      setImageError(
        err?.errors?.user_image?.[0] ?? "画像アップロードに失敗しました。"
      );
    } finally {
      setSubmitLoading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  /* =========================
     Profile Submit
  ========================= */
  const handleProfileUpdate = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!apiClient) return;

    setProfileErrors({});
    setSubmitLoading(true);
    setSuccessMessage("");

    try {
      const res: any = profileUser
        ? await apiClient.patch("/mypage/profile", form)
        : await apiClient.post("/mypage/profile", form);

      initializeProfile(res?.user ?? res?.data?.user ?? null);
      await refresh();

      setSuccessMessage(
        profileUser
          ? "プロフィールを更新しました！"
          : "プロフィールを作成しました！"
      );

      router.replace("/");
    } catch (err: any) {
      if (err?.errors) {
        setProfileErrors(err.errors);
      } else {
        setSuccessMessage("更新時にエラーが発生しました。");
      }
    } finally {
      setSubmitLoading(false);
    }
  };

  /* =========================
     Initial Load（Auth共通仕様）
  ========================= */
  useEffect(() => {
    if (!authReady) return;
    if (!isAuthenticated) return;

    if (!hasFetchedProfile && !isFetching) {
      fetchUserProfile();
    }
  }, [
    authReady,
    isAuthenticated,
    hasFetchedProfile,
    isFetching,
    fetchUserProfile,
  ]);

  /* =========================
     Render Guards（単一）
  ========================= */
  if (!authReady || isAuthLoading || pageLoading) {
    return (
      <div className={`${styles.login_page} max-w-[1400px] mx-auto pt-5 pb-10`}>
        <h2 className={styles.title}>プロフィール設定</h2>
        <h4 className={styles.title_sub}>(プロフィール登録後ご利用になれます。)</h4>
        <div className="text-center p-8">
          <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-red-500 mx-auto"></div>
          <p className="text-gray-500 mt-3">読み込み中...</p>
        </div>
      </div>
    );
  }

  /* =========================
     Render（完全不変）
  ========================= */
  return (
    <div
      className={`${styles.login_page} max-w-[1400px] mx-auto pt-5 pb-10`}
      key={authUser?.id || "unauthenticated"}
    >
      <h2 className={styles.title}>プロフィール設定</h2>

      <div className={styles["form-wrapper"]}>
        {successMessage && (
          <div className={styles["alert-success2"]}>{successMessage}</div>
        )}

        {/* 画像アップロード */}
        <form
          onSubmit={(e) => e.preventDefault()}
          className={styles.item_sell_contents_box_line}
        >
          <div className={styles.image_name}>
            <div className={styles.image_button_row}>
              <img
                src={profileImageUrl}
                alt="プロフィール画像"
                className={styles.user_image_css}
              />
              <button
                type="button"
                className={styles.upload_submit}
                onClick={() => fileInputRef.current?.click()}
                disabled={submitLoading}
              >
                画像を選択する
              </button>
            </div>

            <input
              type="file"
              name="user_image"
              ref={fileInputRef}
              style={{ display: "none" }}
              onChange={handleImageUpload}
              accept="image/*"
            />
          </div>

          <div className={styles.user_image_error_message}>{imageError}</div>
        </form>

        {/* プロフィール更新フォーム */}
        <form onSubmit={handleProfileUpdate}>
          {/* ユーザー名 */}
          <div className={styles["form-group"]}>
            <label htmlFor="display_name" className={styles.label_form_1}>
              ユーザー名
            </label>
            <input
              id="display_name"
              name="display_name"
              type="text"
              className={styles.name_form}
              value={form.display_name}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, display_name: e.target.value }))
              }
            />
            <div className={styles.profile__error}>
              {profileErrors.display_name ? profileErrors.display_name[0] : ""}
            </div>
          </div>

          {/* 郵便番号 */}
          <div className={styles["form-group"]}>
            <label htmlFor="post_number" className={styles.label_form_2}>
              郵便番号 (8桁、ハイフンあり)
            </label>
            <input
              id="post_number"
              type="text"
              className={styles.email_form}
              name="post_number"
              value={form.post_number}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, post_number: e.target.value }))
              }
              placeholder="例: 100-0001"
              maxLength={8}
            />
            <div className={styles.profile__error}>
              {profileErrors.post_number ? profileErrors.post_number[0] : ""}
            </div>
          </div>

          {/* 住所 */}
          <div className={styles["form-group"]}>
            <label htmlFor="address" className={styles.label_form_3}>
              住所
            </label>
            <input
              id="address"
              type="text"
              className={styles.password_form}
              name="address"
              value={form.address}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, address: e.target.value }))
              }
              placeholder="都道府県市区町村を入力してください"
            />
            <div className={styles.profile__error}>
              {profileErrors.address ? profileErrors.address[0] : ""}
            </div>
          </div>

          {/* 建物名 */}
          <div className={styles["form-group"]}>
            <label htmlFor="building" className={styles.label_form_4}>
              建物名
            </label>
            <input
              id="building"
              type="text"
              className={styles.password_form}
              name="building"
              value={form.building}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, building: e.target.value }))
              }
            />
            <div className={styles.profile__error}>
              {profileErrors.building ? profileErrors.building[0] : ""}
            </div>
          </div>

          <div className={styles.submit}>
            <input
              type="submit"
              className={styles.submit_form}
              value="更新する"
              disabled={submitLoading}
            />
          </div>
        </form>
      </div>
    </div>
  );
}
