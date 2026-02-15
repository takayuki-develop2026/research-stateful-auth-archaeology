from __future__ import annotations

import re
from typing import Any, Dict, List, Literal, Optional, TypedDict

from pydantic import BaseModel, Field, ValidationError, field_validator

# RunArtifactContent v1.0 (single source: schema_version=1.0)
SCHEMA_VERSION: Literal["1.0"] = "1.0"

ProducedByType = Literal["system", "user", "job", "tool"]
EvidenceType = Literal["html", "pdf", "image", "text", "api_response", "log", "unknown"]

EVIDENCE_REF_RE = re.compile(r"^(sha256:[a-f0-9]{64}|s3://.+|https://.+|db:.+)$", re.IGNORECASE)
SHA256_RE = re.compile(r"^[a-f0-9]{64}$", re.IGNORECASE)


class ArtifactRef(BaseModel):
    id: str
    kind: str
    run_id: str
    trace_id: str

    model_config = {"extra": "allow"}


class ProducedBy(BaseModel):
    type: ProducedByType
    name: str
    version: Optional[str] = None

    model_config = {"extra": "allow"}


class TraceRef(BaseModel):
    trace_id: str
    span_id: Optional[str] = None
    correlation_id: Optional[str] = None

    model_config = {"extra": "allow"}


class EvidenceRef(BaseModel):
    ref: str
    type: EvidenceType
    sha256: Optional[str] = None
    mime: Optional[str] = None
    size_bytes: Optional[int] = None

    model_config = {"extra": "allow"}

    @field_validator("ref")
    @classmethod
    def validate_ref(cls, v: str) -> str:
        if not v or not EVIDENCE_REF_RE.match(v):
            raise ValueError("evidence_refs.ref must match: sha256:<64hex> | s3://... | https://... | db:...")
        return v

    @field_validator("sha256")
    @classmethod
    def validate_sha256(cls, v: Optional[str]) -> Optional[str]:
        if v is None:
            return v
        if not SHA256_RE.match(v):
            raise ValueError("sha256 must be 64 hex chars")
        return v

    @field_validator("size_bytes")
    @classmethod
    def validate_size_bytes(cls, v: Optional[int]) -> Optional[int]:
        if v is None:
            return v
        if v < 0:
            raise ValueError("size_bytes must be >= 0")
        return v


class RunArtifactContentV1(BaseModel):
    schema_version: Literal["1.0"] = Field(default=SCHEMA_VERSION)
    artifact_ref: ArtifactRef
    produced_by: ProducedBy
    policy_version: str
    pipeline_version: str
    evidence_refs: List[EvidenceRef] = Field(default_factory=list)
    trace: TraceRef

    model_config = {"extra": "allow"}

    @field_validator("policy_version", "pipeline_version")
    @classmethod
    def validate_non_empty(cls, v: str) -> str:
        if not isinstance(v, str) or v.strip() == "":
            raise ValueError("must be non-empty string")
        return v


def validate_run_artifact_content_v1(
    payload: Dict[str, Any],
    *,
    artifact_kind: str,  # run_artifacts.artifact_kind
    run_id: str,         # run_artifacts.run_id
    trace_id: str,       # run_artifacts.trace_id
) -> RunArtifactContentV1:
    """
    One-shot validation entrypoint (call exactly once in the upsert/create usecase).
    Ensures schema + cross-field invariants.
    """
    parsed = RunArtifactContentV1.model_validate(payload)

    # Cross-field invariants (DBでは表現しづらい/壊れやすいので UseCase に1点集中)
    if parsed.artifact_ref.kind != artifact_kind:
        raise ValueError(f"artifact_ref.kind mismatch: {parsed.artifact_ref.kind} != {artifact_kind}")
    if parsed.artifact_ref.run_id != run_id:
        raise ValueError(f"artifact_ref.run_id mismatch: {parsed.artifact_ref.run_id} != {run_id}")
    if parsed.trace.trace_id != trace_id:
        raise ValueError(f"trace.trace_id mismatch: {parsed.trace.trace_id} != {trace_id}")
    if parsed.artifact_ref.trace_id != trace_id:
        raise ValueError(f"artifact_ref.trace_id mismatch: {parsed.artifact_ref.trace_id} != {trace_id}")

    return parsed