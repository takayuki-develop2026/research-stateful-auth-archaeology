"use client";

import useSWR from "swr";
import { useSearchParams } from "next/navigation";
import { RunArtifactFilters } from "@/ui/runArtifacts/RunArtifactFilters";
import { parseRunArtifactQueryFromSearchParams } from "@/domain/runArtifacts/parseRunArtifactQuery";
import { fetchRunArtifacts } from "@/domain/runArtifacts/fetchRunArtifacts";
import { artifactKindLabel } from "@/domain/contracts/runArtifacts/v1/artifactKinds";

const BASE_URL = ""; // same-origin

export default function RunArtifactsPage() {
  const sp = useSearchParams();
  const query = parseRunArtifactQueryFromSearchParams(
    new URLSearchParams(sp.toString()),
  );

  const key = [
    "run-artifacts",
    query.kind ?? "",
    query.q ?? "",
    query.page ?? 1,
  ];

  const { data, error, isLoading } = useSWR(key, async () =>
    fetchRunArtifacts(BASE_URL, query),
  );

  return (
    <div style={{ padding: 16 }}>
      <h1 style={{ fontSize: 20, marginBottom: 12 }}>Run Artifacts</h1>

      <RunArtifactFilters />

      {isLoading && <p style={{ marginTop: 12 }}>Loading…</p>}
      {error && (
        <p style={{ marginTop: 12, color: "crimson" }}>{String(error)}</p>
      )}

      {data && (
        <div style={{ marginTop: 12, borderTop: "1px solid #eee" }}>
          {data.items.map((it) => (
            <div
              key={`${it.run_id}:${it.artifact_kind}:${it.created_at}`}
              style={{ padding: 12, borderBottom: "1px solid #eee" }}
            >
              <div style={{ fontSize: 12, color: "#666" }}>{it.created_at}</div>
              <div style={{ fontSize: 14 }}>
                <b>{artifactKindLabel(it.artifact_kind)}</b> — run_id:{" "}
                {it.run_id}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
