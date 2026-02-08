"use client";

import React, { useMemo } from "react";
import dynamic from "next/dynamic";

export { useAuth } from "@/ui/auth/core/AuthContextCore";

// ─────────────────────────────────────────────
// Debug logger (dev only)
// ─────────────────────────────────────────────
const DEV = process.env.NODE_ENV !== "production";
const log = (...args: any[]) => {
  if (DEV) console.warn(...args);
};

// ✅ module load（dev only）
log("[AuthProvider] module loaded");

// ─────────────────────────────────────────────
// Dynamic providers (client only)
// ─────────────────────────────────────────────
const SanctumProvider = dynamic(() => import("./modes/SanctumProvider"), {
  ssr: false,
  loading: () => {
    log("[AuthProvider] loading SanctumProvider...");
    return null;
  },
});

const FirebaseJwtProvider = dynamic(
  () => import("./modes/FirebaseJwtProvider"),
  {
    ssr: false,
    loading: () => {
      log("[AuthProvider] loading FirebaseJwtProvider...");
      return null;
    },
  },
);

const IdaasProvider = dynamic(() => import("./modes/IdaasProvider"), {
  ssr: false,
  loading: () => {
    log("[AuthProvider] loading IdaasProvider...");
    return null;
  },
});

type AuthMode = "sanctum" | "idaas" | "firebase_jwt";

function resolveMode(raw: string | undefined | null): AuthMode {
  if (raw === "sanctum") return "sanctum";
  if (raw === "idaas") return "idaas";
  return "firebase_jwt";
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  // NEXT_PUBLIC_* はビルド時に埋め込まれ、ランタイムで undefined になるケースもあるので保険
  const raw = process.env.NEXT_PUBLIC_AUTH_MODE;

  const mode = useMemo<AuthMode>(() => resolveMode(raw), [raw]);

  // ✅ render（dev only）
  log("[AuthProvider] render", { raw, mode });

  switch (mode) {
    case "firebase_jwt":
      log("[AuthProvider] -> FirebaseJwtProvider");
      return <FirebaseJwtProvider>{children}</FirebaseJwtProvider>;

    case "idaas":
      log("[AuthProvider] -> IdaasProvider");
      return <IdaasProvider>{children}</IdaasProvider>;

    case "sanctum":
      log("[AuthProvider] -> SanctumProvider");
      return <SanctumProvider>{children}</SanctumProvider>;

    default:
      // 型的に来ないが、安全側
      log("[AuthProvider] -> fallback FirebaseJwtProvider");
      return <FirebaseJwtProvider>{children}</FirebaseJwtProvider>;
  }
}
