"use client";

import React from "react";
import dynamic from "next/dynamic";

export { useAuth } from "@/ui/auth/core/AuthContextCore";

// ✅ module load（このファイルが読み込まれたら必ず出る）
console.warn("[AuthProvider] module loaded");

const SanctumProvider = dynamic(() => import("./modes/SanctumProvider"), {
  ssr: false,
  loading: () => {
    console.warn("[AuthProvider] loading SanctumProvider...");
    return null;
  },
});

const FirebaseJwtProvider = dynamic(
  () => import("./modes/FirebaseJwtProvider"),
  {
    ssr: false,
    loading: () => {
      console.warn("[AuthProvider] loading FirebaseJwtProvider...");
      return null;
    },
  },
);

const IdaasProvider = dynamic(() => import("./modes/IdaasProvider"), {
  ssr: false,
  loading: () => {
    console.warn("[AuthProvider] loading IdaasProvider...");
    return null;
  },
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const raw = process.env.NEXT_PUBLIC_AUTH_MODE;

  const mode =
    raw === "sanctum" ? "sanctum" : raw === "idaas" ? "idaas" : "firebase_jwt";

  // ✅ render（このコンポーネントがレンダーされるたびに出る）
  console.warn("[AuthProvider] render", { raw, mode });

  if (mode === "firebase_jwt") {
    console.warn("[AuthProvider] -> FirebaseJwtProvider");
    return <FirebaseJwtProvider>{children}</FirebaseJwtProvider>;
  }

  if (mode === "idaas") {
    console.warn("[AuthProvider] -> IdaasProvider");
    return <IdaasProvider>{children}</IdaasProvider>;
  }

  console.warn("[AuthProvider] -> SanctumProvider");
  return <SanctumProvider>{children}</SanctumProvider>;
}
