// frontend/src/domain/contracts/runArtifacts/v1/artifactKinds.ts

/**
 * ArtifactKind catalog for UI (known kinds only).
 * - UI uses this for dropdown/filter labels.
 * - Server may return unknown kinds; we must accept them as long as format is valid.
 */

export const ARTIFACT_KINDS = {
  EVIDENCE_ASSET: "evidence.asset",
  DIFF_JSON: "diff.json",
  REVIEW_SNAPSHOT_BEFORE: "review.snapshot.before",
  REVIEW_SNAPSHOT_AFTER: "review.snapshot.after",
  DECISION_PAYLOAD: "decision.payload",
  POLICY_EVALUATION: "policy.evaluation",
} as const;

export type KnownArtifactKind =
  (typeof ARTIFACT_KINDS)[keyof typeof ARTIFACT_KINDS];

/**
 * Format: lowercase alnum + dot segments (same as Laravel side).
 * Examples:
 * - evidence.asset
 * - diff.json
 * - review.snapshot.before
 */
export const ARTIFACT_KIND_FORMAT_RE = /^[a-z0-9]+(\.[a-z0-9]+)*$/;

/**
 * UI label map for known kinds.
 * Unknown kinds should fall back to the raw string.
 */
export const ARTIFACT_KIND_LABELS: Record<KnownArtifactKind, string> = {
  [ARTIFACT_KINDS.EVIDENCE_ASSET]: "Evidence Asset",
  [ARTIFACT_KINDS.DIFF_JSON]: "Diff (JSON)",
  [ARTIFACT_KINDS.REVIEW_SNAPSHOT_BEFORE]: "Review Snapshot (Before)",
  [ARTIFACT_KINDS.REVIEW_SNAPSHOT_AFTER]: "Review Snapshot (After)",
  [ARTIFACT_KINDS.DECISION_PAYLOAD]: "Decision Payload",
  [ARTIFACT_KINDS.POLICY_EVALUATION]: "Policy Evaluation",
};

/**
 * List options for dropdowns.
 */
export function listKnownArtifactKindOptions(): Array<{
  value: KnownArtifactKind;
  label: string;
}> {
  const values = Object.values(ARTIFACT_KINDS);
  return values.map((v) => ({
    value: v,
    label: ARTIFACT_KIND_LABELS[v],
  }));
}

/**
 * Accept unknown kinds as long as the format is valid.
 * Use this at UI boundary (router params / API responses / user input).
 */
export function isValidArtifactKind(kind: string): boolean {
  if (!kind) return false;
  return ARTIFACT_KIND_FORMAT_RE.test(kind);
}

/**
 * For display: known kinds -> label; unknown -> raw.
 */
export function artifactKindLabel(kind: string): string {
  if (!kind) return "";
  const known = (Object.values(ARTIFACT_KINDS) as string[]).includes(kind);
  return known ? ARTIFACT_KIND_LABELS[kind as KnownArtifactKind] : kind;
}

/**
 * Normalize: trim + lowercase (optional).
 * NOTE: Only do this if your backend assumes lowercase canonicalization.
 * If backend is strict, normalization helps avoid accidental mismatches.
 */
export function normalizeArtifactKind(kind: string): string {
  return (kind ?? "").trim().toLowerCase();
}
