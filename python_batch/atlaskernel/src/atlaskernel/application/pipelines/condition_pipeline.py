from __future__ import annotations

from typing import Any, Dict, List, Optional, Tuple

from atlaskernel.services.normalize import normalize, normalize_key
from atlaskernel.services.similarity import similarity
from atlaskernel.adapters.assets_loader import load_assets
from atlaskernel.domain.candidate import Candidate as InternalCandidate
from atlaskernel.domain.result import AnalysisResult, Candidate as ResultCandidate
from atlaskernel.version import VERSION


def _parse_canon_tsv_line(line: str) -> Optional[Tuple[str, str, List[str]]]:
    """
    conditions_canon_v1.tsv:
      canonical<TAB>normalized_key<TAB>aliases(comma-separated)
    """
    s = line.strip()
    if not s or s.startswith("#"):
        return None
    parts = s.split("\t")
    canonical = parts[0].strip() if len(parts) >= 1 else ""
    if not canonical:
        return None
    normalized = parts[1].strip() if len(parts) >= 2 and parts[1].strip() else normalize_key(canonical)
    aliases_raw = parts[2].strip() if len(parts) >= 3 else ""
    aliases = [a.strip() for a in aliases_raw.split(",") if a.strip()] if aliases_raw else []
    return canonical, normalized, aliases


def _load_canon_defs(ref: str) -> List[Tuple[str, str, List[str]]]:
    lines = load_assets(ref)
    out: List[Tuple[str, str, List[str]]] = []
    for line in lines:
        parsed = _parse_canon_tsv_line(line)
        if parsed:
            out.append(parsed)
    return out


def _build_alias_map_from_canon(canon_defs: List[Tuple[str, str, List[str]]]) -> Dict[str, str]:
    m: Dict[str, str] = {}
    for canonical, _key, aliases in canon_defs:
        m[normalize(canonical)] = canonical
        for a in aliases:
            m[normalize(a)] = canonical
    return m


def _load_alias_map(ref: str) -> Dict[str, str]:
    """
    conditions_alias_v1.tsv:
      alias<TAB>canonical
    """
    lines = load_assets(ref)
    m: Dict[str, str] = {}
    for line in lines:
        s = line.strip()
        if not s or s.startswith("#"):
            continue
        if "\t" not in s:
            continue
        alias, canonical = s.split("\t", 1)
        a = normalize(alias)
        c = canonical.strip()
        if a and c:
            m[a] = c
    return m


def analyze_condition(request, policy_engine, ctx=None) -> AnalysisResult:
    norm = normalize(request.raw_value)

    # 0) canonical SoT
    canon_defs = _load_canon_defs("conditions_canon_v1")

    # 1) alias -> canonical
    alias_map = _load_alias_map("conditions_alias_v1")
    if not alias_map and canon_defs:
        alias_map = _build_alias_map_from_canon(canon_defs)

    alias_hit = alias_map.get(norm)
    has_alias = bool(alias_hit)

    internal: List[InternalCandidate] = []
    explanation: List[Dict[str, Any]] = []

    if alias_hit:
        top_value = alias_hit
        top_score = 0.95
        internal = [InternalCandidate(value=top_value, score=float(top_score))]
        explanation.append({
            "rule": "alias_map",
            "detail": f"alias hit ({request.raw_value} -> {alias_hit})",
            "trace": {"alias_hit": True, "alias_canonical": alias_hit},
        })
    else:
        canonicals = [c for (c, _k, _a) in canon_defs] if canon_defs else []
        if not canonicals:
            canonicals = load_assets(request.known_assets_ref or "conditions_v1")

        for c in canonicals:
            score = similarity(norm, normalize(c))
            internal.append(InternalCandidate(value=c, score=float(score)))

        if not internal:
            raise RuntimeError("No condition assets loaded.")

        internal.sort(key=lambda c: c.score, reverse=True)
        top = internal[0]
        top_value = top.value
        top_score = float(top.score)

        explanation.append({
            "rule": "similarity",
            "detail": f"top={top_score}",
            "trace": {"top_raw": top.value},
        })

    # policy
    decision, reason, trace = policy_engine.evaluate(
        policy_engine.load("condition"),
        {"score": float(top_score)},
    )
    rule_id = (trace.get("rule_id") if isinstance(trace, dict) else None) or ("alias_map" if has_alias else "policy")

    explanation.append({
        "rule": "policy",
        "detail": reason or "n/a",
        "trace": trace,
    })

    result_candidates: List[ResultCandidate] = [
        ResultCandidate(value=c.value, score=float(c.score))
        for c in (internal[:5] if internal else [])
    ]
    if not result_candidates:
        result_candidates = [ResultCandidate(value=top_value, score=float(top_score))]

    extensions: Dict[str, Any] = {
        "policy_trace": trace,
        "has_alias": has_alias,
    }
    if has_alias:
        extensions["alias_hit"] = True
        extensions["alias_canonical"] = alias_hit

    if decision in ("needs_review", "rejected"):
        extensions["escalation"] = {"action": "human_review", "queue": "entity_review.condition"}

    return AnalysisResult(
        entity_type="condition",
        raw_value=request.raw_value,
        canonical_value=top_value,
        confidence=float(top_score),
        decision=decision,
        rule_id=rule_id,
        candidates=result_candidates,
        explanation=explanation,
        engine_version=VERSION,
        extensions=extensions,
    )