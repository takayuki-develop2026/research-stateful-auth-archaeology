import type { AuthClient, LoginResult, RegisterResult } from "./AuthClient";
import type { AuthUser } from "@/domain/auth/AuthUser";

export type AxiosLikeClient = {
  get<T = any>(url: string): Promise<T>;
  post<T = any>(url: string, body?: any): Promise<T>;
  patch<T = any>(url: string, body?: any): Promise<T>;
  delete<T = any>(url: string): Promise<T>;
};

export interface AuthContextType {
  user: AuthUser | null;
  isAuthenticated: boolean;

  isLoading: boolean;
  isReady: boolean;

  authClient: AuthClient;
  apiClient: AxiosLikeClient;

  login(args: { email: string; password: string }): Promise<LoginResult>;
  register(args: {
    name: string;
    email: string;
    password: string;
  }): Promise<RegisterResult>;
  logout(): Promise<void>;
  reloadUser(): Promise<void>;
  reloginWithFirebaseToken(idToken: string): Promise<void>;

  // ---- backward-compat（既存コード互換）----
  authReady?: boolean;
  isAuthLoading?: boolean;
  refresh?: () => Promise<void>;
}
