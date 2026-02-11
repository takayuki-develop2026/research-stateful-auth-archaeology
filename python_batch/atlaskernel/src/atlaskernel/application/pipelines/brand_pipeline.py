from __future__ import annotations

from typing import Dict, List, Optional

from atlaskernel.domain.request import AnalysisRequest
from atlaskernel.domain.candidate import Candidate as InternalCandidate
from atlaskernel.domain.result import AnalysisResult, Candidate as ResultCandidate
from atlaskernel.services.normalize import normalize
from atlaskernel.services.similarity import similarity
from atlaskernel.adapters.assets_loader import load_assets
from atlaskernel.version import VERSION

_brand_alias_map_cache: Optional[Dict[str, str]] = None
_brand_alias_keys_cache: Optional[List[str]] = None


def _load_brand_alias_map(ref: str = "brands_alias_v1") -> Dict[str, str]:
    global _brand_alias_map_cache, _brand_alias_keys_cache
    if _brand_alias_map_cache is not None:
        return _brand_alias_map_cache

    m: Dict[str, str] = {}
    lines = load_assets(ref)
    for line in lines:
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        if "\t" not in s:
            continue
        alias, canon = s.split("\t", 1)
        a = normalize(alias)
        c = canon.strip()
        if a and c:
            m[a] = c

    _brand_alias_map_cache = m
    _brand_alias_keys_cache = sorted(m.keys(), key=len, reverse=True)
    return m


def _alias_exact(norm: str) -> Optional[str]:
    m = _load_brand_alias_map()
    return m.get(norm)


def _alias_prefix(norm: str) -> Optional[str]:
    m = _load_brand_alias_map()
    keys = _brand_alias_keys_cache or []
    for k in keys:
        if norm.startswith(k):
            return m.get(k)
    return None


def _canonicalize_any(s: str) -> str:
    n = normalize(s)
    return _alias_exact(n) or _alias_prefix(n) or s


def analyze_brand(req: AnalysisRequest, policy_engine, ctx=None) -> AnalysisResult:
    """
    ✅ Contract:
      - candidates: List[ResultCandidate]
      - extensions.policy_trace 必須
      - needs_review / rejected なら extensions.escalation 必須
      - brand は has_alias を policy input に渡す（強推奨）
    """
    norm = normalize(req.raw_value)

    # alias fast path
    canonical = _alias_exact(norm) or _alias_prefix(norm)
    has_alias = bool(canonical)

    internal: List[InternalCandidate] = []
    if canonical:
        top_value = canonical
        top_score = 0.95
        internal = [InternalCandidate(value=canonical, score=float(top_score))]
        explanation: List[dict] = [{
            "rule": "alias_map",
            "detail": f"alias hit ({req.raw_value} -> {canonical})",
            "trace": {"alias_hit": True, "alias_canonical": canonical},
        }]
    else:
        assets = load_assets(req.known_assets_ref or "brands_v1")
        for a in assets:
            score = similarity(norm, normalize(a))
            internal.append(InternalCandidate(value=a, score=float(score)))

        internal.sort(key=lambda c: c.score, reverse=True)
        top = internal[0] if internal else InternalCandidate(value=req.raw_value, score=0.0)

        top_value = _canonicalize_any(top.value)
        top_score = float(top.score)

        explanation = [{
            "rule": "similarity",
            "detail": f"top={top_score}",
            "trace": {"top_raw": top.value, "top_canonicalized": top_value},
        }]

    # policy
    decision, reason, trace = policy_engine.evaluate(
        policy_engine.load("brand"),
        {"score": float(top_score), "has_alias": has_alias},
    )

    rule_id = (trace.get("rule_id") if isinstance(trace, dict) else None) or ("alias_map" if canonical else "policy")

    explanation.append({
        "rule": "policy",
        "detail": reason or "n/a",
        "trace": trace,
    })

    # candidates (output)
    result_candidates: List[ResultCandidate] = [
        ResultCandidate(value=_canonicalize_any(c.value), score=float(c.score))
        for c in (internal[:5] if internal else [])
    ]
    if not result_candidates:
        result_candidates = [ResultCandidate(value=top_value, score=float(top_score))]

    extensions: Dict[str, Any] = {  # type: ignore[name-defined]
        "policy_trace": trace,
        "has_alias": has_alias,
    }
    if canonical:
        extensions["alias_hit"] = True
        extensions["alias_canonical"] = canonical

    if decision in ("needs_review", "rejected"):
        extensions["escalation"] = {"action": "human_review", "queue": "entity_review.brand"}

    return AnalysisResult(
        entity_type="brand",
        raw_value=req.raw_value,
        canonical_value=top_value,
        confidence=float(top_score),
        decision=decision,
        rule_id=rule_id,
        candidates=result_candidates,
        explanation=explanation,
        engine_version=VERSION,
        extensions=extensions,
    )