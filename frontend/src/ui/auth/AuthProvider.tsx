"use client";

import React from "react";
import dynamic from "next/dynamic";

export { useAuth } from "@/ui/auth/core/AuthContextCore";

const SanctumProvider = dynamic(() => import("./modes/SanctumProvider"), {
  ssr: false,
});

const FirebaseJwtProvider = dynamic(
  () => import("./modes/FirebaseJwtProvider"),
  { ssr: false },
);

const IdaasProvider = dynamic(() => import("./modes/IdaasProvider"), {
  ssr: false,
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const mode =
    process.env.NEXT_PUBLIC_AUTH_MODE === "sanctum"
      ? "sanctum"
      : process.env.NEXT_PUBLIC_AUTH_MODE === "idaas"
        ? "idaas"
        : "firebase_jwt";

  if (mode === "firebase_jwt")
    return <FirebaseJwtProvider>{children}</FirebaseJwtProvider>;
  if (mode === "idaas") return <IdaasProvider>{children}</IdaasProvider>;
  return <SanctumProvider>{children}</SanctumProvider>;
}
