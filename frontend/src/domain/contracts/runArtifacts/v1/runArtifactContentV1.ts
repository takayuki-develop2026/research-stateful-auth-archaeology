import { z } from "zod";

/**
 * RunArtifactContent v1.0 (single source: schema_version=1.0)
 * - Keep keys/enums identical to:
 *   - contracts/run_artifacts/run_artifact.schema.v1.json
 *   - python_atlaskernel/atlaskernel/domain/contracts/run_artifact_v1.py
 */

export const SCHEMA_VERSION = "1.0" as const;

export const ProducedByType = z.enum(["system", "user", "job", "tool"]);
export type ProducedByType = z.infer<typeof ProducedByType>;

export const EvidenceType = z.enum([
  "html",
  "pdf",
  "image",
  "text",
  "api_response",
  "log",
  "unknown",
]);
export type EvidenceType = z.infer<typeof EvidenceType>;

export const EvidenceRefPattern =
  /^(sha256:[a-f0-9]{64}|s3:\/\/.+|https:\/\/.+|db:.+)$/i;

export const ArtifactRefSchema = z
  .object({
    id: z.string().min(1),
    kind: z.string().min(1),
    run_id: z.string().min(1),
    trace_id: z.string().min(1),
  })
  .passthrough();

export const ProducedBySchema = z
  .object({
    type: ProducedByType,
    name: z.string().min(1),
    version: z.string().min(1).optional(),
  })
  .passthrough();

export const TraceSchema = z
  .object({
    trace_id: z.string().min(1),
    span_id: z.string().min(1).optional(),
    correlation_id: z.string().min(1).optional(),
  })
  .passthrough();

export const EvidenceRefSchema = z
  .object({
    ref: z.string().min(1).regex(EvidenceRefPattern),
    type: EvidenceType,
    sha256: z
      .string()
      .regex(/^[a-f0-9]{64}$/i)
      .optional(),
    mime: z.string().min(1).optional(),
    size_bytes: z.number().int().nonnegative().optional(),
  })
  .passthrough();

export const RunArtifactContentV1Schema = z
  .object({
    schema_version: z.literal(SCHEMA_VERSION),
    artifact_ref: ArtifactRefSchema,
    produced_by: ProducedBySchema,
    policy_version: z.string().min(1),
    pipeline_version: z.string().min(1),
    evidence_refs: z.array(EvidenceRefSchema),
    trace: TraceSchema,
  })
  .passthrough();

export type ArtifactRef = z.infer<typeof ArtifactRefSchema>;
export type ProducedBy = z.infer<typeof ProducedBySchema>;
export type TraceRef = z.infer<typeof TraceSchema>;
export type EvidenceRef = z.infer<typeof EvidenceRefSchema>;
export type RunArtifactContentV1 = z.infer<typeof RunArtifactContentV1Schema>;

export type ValidateParams = {
  artifact_kind: string; // run_artifacts.artifact_kind
  run_id: string; // run_artifacts.run_id
  trace_id: string; // run_artifacts.trace_id
};

/**
 * One-shot validation entrypoint (recommended usage:
 * call exactly once in the "create/upsert artifact" usecase).
 */
export function validateRunArtifactContentV1(
  input: unknown,
  p: ValidateParams,
): RunArtifactContentV1 {
  const parsed = RunArtifactContentV1Schema.parse(input);

  // Cross-field invariants (DBでは表現しづらい/壊れやすいので UseCase に1点集中)
  if (parsed.artifact_ref.kind !== p.artifact_kind) {
    throw new Error(
      `artifact_ref.kind mismatch: ${parsed.artifact_ref.kind} != ${p.artifact_kind}`,
    );
  }
  if (parsed.artifact_ref.run_id !== p.run_id) {
    throw new Error(
      `artifact_ref.run_id mismatch: ${parsed.artifact_ref.run_id} != ${p.run_id}`,
    );
  }
  if (parsed.trace.trace_id !== p.trace_id) {
    throw new Error(
      `trace.trace_id mismatch: ${parsed.trace.trace_id} != ${p.trace_id}`,
    );
  }
  if (parsed.artifact_ref.trace_id !== p.trace_id) {
    throw new Error(
      `artifact_ref.trace_id mismatch: ${parsed.artifact_ref.trace_id} != ${p.trace_id}`,
    );
  }

  return parsed;
}
