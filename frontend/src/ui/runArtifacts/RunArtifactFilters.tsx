"use client";

import { useMemo, useCallback } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  listKnownArtifactKindOptions,
  normalizeArtifactKind,
  isValidArtifactKind,
} from "@/domain/contracts/runArtifacts/v1/artifactKinds";

type Props = {
  /**
   * Optional: if you want to override where filters live (default: current path).
   * Example: "/admin/run-artifacts"
   */
  basePath?: string;

  /**
   * Optional: if you want to control default placeholder text.
   */
  searchPlaceholder?: string;
};

function clampStr(v: string | null, max = 200): string {
  if (!v) return "";
  const s = v.trim();
  if (s.length > max) return s.slice(0, max);
  return s;
}

export function RunArtifactFilters(props: Props) {
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();

  const basePath = props.basePath ?? pathname;
  const searchPlaceholder =
    props.searchPlaceholder ?? "Search… (run_id / trace_id / kind / etc)";

  const kindRaw = sp.get("kind");
  const qRaw = sp.get("q");

  const kind = useMemo(() => {
    const k = normalizeArtifactKind(kindRaw ?? "");
    if (!k) return "";
    return isValidArtifactKind(k) ? k : ""; // invalid => drop
  }, [kindRaw]);

  const q = useMemo(() => clampStr(qRaw), [qRaw]);

  const kindOptions = useMemo(() => {
    return [
      { value: "", label: "All kinds" },
      ...listKnownArtifactKindOptions(),
    ];
  }, []);

  const pushParams = useCallback(
    (next: { kind?: string; q?: string; page?: string }) => {
      const p = new URLSearchParams(sp.toString());

      // kind
      if (next.kind !== undefined) {
        const nk = normalizeArtifactKind(next.kind);
        if (nk && isValidArtifactKind(nk)) p.set("kind", nk);
        else p.delete("kind");
      }

      // q
      if (next.q !== undefined) {
        const nq = clampStr(next.q);
        if (nq) p.set("q", nq);
        else p.delete("q");
      }

      // page (optional): whenever filters change, reset page to 1
      if (next.page !== undefined) {
        if (next.page) p.set("page", next.page);
        else p.delete("page");
      }

      const qs = p.toString();
      router.replace(qs ? `${basePath}?${qs}` : basePath);
    },
    [router, basePath, sp],
  );

  return (
    <div
      style={{
        display: "flex",
        gap: 12,
        alignItems: "center",
        flexWrap: "wrap",
      }}
    >
      <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <span style={{ fontSize: 12, color: "#666" }}>Kind</span>
        <select
          value={kind}
          onChange={(e) => pushParams({ kind: e.target.value, page: "" })}
          style={{ padding: 8, minWidth: 240 }}
        >
          {kindOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>

      <label style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <span style={{ fontSize: 12, color: "#666" }}>Search</span>
        <input
          value={q}
          onChange={(e) => pushParams({ q: e.target.value, page: "" })}
          placeholder={searchPlaceholder}
          style={{ padding: 8, minWidth: 360 }}
        />
      </label>

      <button
        type="button"
        onClick={() => pushParams({ kind: "", q: "", page: "" })}
        style={{
          padding: "10px 12px",
          border: "1px solid #ddd",
          background: "#fff",
          cursor: "pointer",
        }}
      >
        Reset
      </button>
    </div>
  );
}
