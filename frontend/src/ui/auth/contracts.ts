import type { AuthUser } from "@/domain/auth/AuthUser";

/* =========================
   API Client Contract
========================= */
export interface ApiClient {
  get<T>(url: string): Promise<T>;
  post<T>(url: string, body?: unknown): Promise<T>;
  patch<T>(url: string, body?: unknown): Promise<T>;
  delete<T>(url: string): Promise<T>;
}

/* =========================
   Login Payload (multi-mode)
========================= */
export type PasswordLoginPayload = {
  type: "password";
  email: string;
  password: string;
};

export type OidcLoginPayload = {
  type: "oidc";
  returnTo?: string; // 例: "/mypage/profile"
};

export type LoginPayload = PasswordLoginPayload | OidcLoginPayload;

/* =========================
   Auth Context
========================= */
export type AuthContext = {
  isLoading: boolean;
  authReady: boolean;
  isAuthenticated: boolean;
  user: AuthUser | null;
  apiClient: ApiClient;

  login(payload: LoginPayload): Promise<void>;
  logout(): Promise<void>;
  refresh(): Promise<void>;
};

/* =========================
   Auth Adapter
========================= */
export interface AuthAdapter {
  init(): Promise<AuthUser | null>;
  login(payload: LoginPayload): Promise<void>;
  logout(): Promise<void>;
  getApiClient(): ApiClient;
}
