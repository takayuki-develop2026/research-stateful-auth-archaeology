import {
  isValidArtifactKind,
  normalizeArtifactKind,
} from "@/domain/contracts/runArtifacts/v1/artifactKinds";

export type RunArtifactQuery = {
  kind?: string; // valid format only
  q?: string; // trimmed
  page?: number; // >=1
};

function clampStr(v: string | null, max = 200): string | undefined {
  if (!v) return undefined;
  const s = v.trim();
  if (!s) return undefined;
  return s.length > max ? s.slice(0, max) : s;
}

function clampPage(v: string | null): number | undefined {
  if (!v) return undefined;
  const n = Number(v);
  if (!Number.isFinite(n)) return undefined;
  const i = Math.floor(n);
  if (i < 1) return 1;
  if (i > 9999) return 9999;
  return i;
}

export function parseRunArtifactQueryFromSearchParams(
  sp: URLSearchParams,
): RunArtifactQuery {
  const kindRaw = sp.get("kind");
  const qRaw = sp.get("q");
  const pageRaw = sp.get("page");

  const kindNorm = kindRaw ? normalizeArtifactKind(kindRaw) : "";
  const kind = kindNorm && isValidArtifactKind(kindNorm) ? kindNorm : undefined;

  const q = clampStr(qRaw);
  const page = clampPage(pageRaw);

  return { kind, q, page };
}
