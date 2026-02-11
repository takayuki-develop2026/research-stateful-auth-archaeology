from __future__ import annotations

from typing import Any, Dict, List, Optional
from pydantic import BaseModel, Field


class Candidate(BaseModel):
    value: str
    score: float


class AnalysisResult(BaseModel):
    """
    ✅ pipeline の統一戻り値 contract（brand/condition/color 共通）
    """
    entity_type: str
    raw_value: str
    canonical_value: Optional[str] = None
    confidence: float = 0.0

    # decision: "auto_accept" | "needs_review" | "rejected"
    decision: str = "rejected"
    rule_id: str = "unknown"

    candidates: List[Candidate] = Field(default_factory=list)

    # explanation: [{"rule": "...", "detail": "...", "trace": {...}}, ...]
    explanation: List[Dict[str, Any]] = Field(default_factory=list)

    # extensions: {"policy_trace": {...}, "escalation": {...}, "has_alias": bool, ...}
    extensions: Dict[str, Any] = Field(default_factory=dict)

    # reproducibility
    engine_version: str = "0.0.0"