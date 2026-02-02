"use client";

import React, { useEffect, useMemo, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import type { AuthContext } from "@/ui/auth/contracts";
import type { AuthUser } from "@/domain/auth/AuthUser";
import { AuthContextCoreProvider } from "@/ui/auth/core/AuthContextCore";
import { SanctumAuthAdapter } from "@/ui/auth/sanctum/SanctumAuthAdapter";

export default function SanctumProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const adapter = useMemo(() => new SanctumAuthAdapter(), []);

  const [isLoading, setIsLoading] = useState(true);
  const [authReady, setAuthReady] = useState(false);
  const [user, setUser] = useState<AuthUser | null>(null);

  useEffect(() => {
    let mounted = true;

    (async () => {
      try {
        const u = await adapter.init(); // あなたの adapter 仕様に合わせる
        if (mounted) setUser(u);
      } finally {
        if (mounted) {
          setIsLoading(false);
          setAuthReady(true);
        }
      }
    })();

    return () => {
      mounted = false;
    };
  }, [adapter]);

  const refresh = useCallback(async () => {
    const u = await adapter.init();
    setUser(u);
  }, [adapter]);

  const value: AuthContext = useMemo(
    () => ({
      isLoading,
      authReady,
      isAuthenticated: !!user,
      user,
      apiClient: adapter.getApiClient(),

      login: async ({ email, password }) => {
        await adapter.login({ email, password });
        await refresh();
      },

      logout: async () => {
        await adapter.logout();
        setUser(null);
        router.replace("/login");
      },

      refresh,
    }),
    [adapter, isLoading, authReady, user, router, refresh],
  );

  return (
    <AuthContextCoreProvider value={value}>{children}</AuthContextCoreProvider>
  );
}
