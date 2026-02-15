import type { RunArtifactQuery } from "@/domain/runArtifacts/parseRunArtifactQuery";

export type RunArtifactListItem = {
  run_id: string;
  artifact_kind: string;
  created_at: string;
  // ... add what your API returns
};

export type RunArtifactListResponse = {
  items: RunArtifactListItem[];
  next_cursor?: string | null;
  // or page-based: total/page/per_page etc
};

export async function fetchRunArtifacts(
  baseUrl: string,
  query: RunArtifactQuery,
): Promise<RunArtifactListResponse> {
  const sp = new URLSearchParams();
  if (query.kind) sp.set("kind", query.kind);
  if (query.q) sp.set("q", query.q);
  if (query.page) sp.set("page", String(query.page));

  const url = `${baseUrl}/api/run-artifacts${sp.toString() ? `?${sp.toString()}` : ""}`;

  const res = await fetch(url, {
    method: "GET",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`fetchRunArtifacts failed: ${res.status}`);
  return res.json();
}
