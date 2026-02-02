"use client";

import React, {
  useEffect,
  useMemo,
  useRef,
  useState,
  useCallback,
} from "react";
import { useRouter } from "next/navigation";
import type { AuthContext } from "@/ui/auth/contracts";
import type { AuthUser } from "@/domain/auth/AuthUser";
import { AuthContextCoreProvider } from "@/ui/auth/core/AuthContextCore";

import { AuthService } from "@/application/auth/AuthService";
import { FirebaseAuthClient } from "@/infrastructure/auth/FirebaseAuthClient";
import { LaravelAuthApi } from "@/infrastructure/auth/LaravelAuthApi";
import { createHttpClient } from "@/infrastructure/auth/HttpClient";
import { TokenStorage } from "@/infrastructure/auth/TokenStorage";
import { createFirebaseJwtApiClient } from "@/ui/auth/firebaseJwt/FirebaseJwtApiClient";

export default function FirebaseJwtProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();

  const [isLoading, setIsLoading] = useState(true);
  const [authReady, setAuthReady] = useState(false);
  const [user, setUser] = useState<AuthUser | null>(null);

  const authServiceRef = useRef<AuthService | null>(null);

  const apiClient = useMemo(() => createFirebaseJwtApiClient(), []);

  const refresh = useCallback(async () => {
    try {
      const u = await apiClient.get<AuthUser>("/me");
      setUser(u);
    } catch {
      setUser(null);
    }
  }, [apiClient]);

  useEffect(() => {
    const firebase = new FirebaseAuthClient();

    const api = new LaravelAuthApi(null);
    const client = createHttpClient(null);
    api.setClient(client);

    const auth = new AuthService(firebase, api);
    authServiceRef.current = auth;

    const { accessToken } = TokenStorage.load();
    if (!accessToken) {
      setIsLoading(false);
      setAuthReady(true);
      return;
    }

    (async () => {
      try {
        await refresh();
      } catch {
        TokenStorage.clear();
        setUser(null);
      } finally {
        setIsLoading(false);
        setAuthReady(true);
      }
    })();
  }, [apiClient, refresh]);

  const value: AuthContext = useMemo(
    () => ({
      isLoading,
      authReady,
      isAuthenticated: !!user,
      user,
      apiClient,

      login: async ({ email, password }) => {
        const auth = authServiceRef.current;
        if (!auth) throw new Error("AuthService not ready");

        setIsLoading(true);
        try {
          await auth.login({ email, password });
          await refresh();
        } finally {
          setIsLoading(false);
        }
      },

      logout: async () => {
        const auth = authServiceRef.current;
        if (auth) await auth.logout();
        TokenStorage.clear();
        setUser(null);
        router.replace("/login");
      },

      refresh,
    }),
    [isLoading, authReady, user, apiClient, router, refresh],
  );

  return (
    <AuthContextCoreProvider value={value}>{children}</AuthContextCoreProvider>
  );
}
