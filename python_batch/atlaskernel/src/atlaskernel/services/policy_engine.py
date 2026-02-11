from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Dict, Optional, Tuple

import yaml
from importlib import resources


def _deep_merge(base: Dict[str, Any], child: Dict[str, Any]) -> Dict[str, Any]:
    """
    child wins. dict-dict は再帰マージ。
    """
    out: Dict[str, Any] = dict(base or {})
    for k, v in (child or {}).items():
        if isinstance(v, dict) and isinstance(out.get(k), dict):
            out[k] = _deep_merge(out[k], v)
        else:
            out[k] = v
    return out


@dataclass(frozen=True)
class PolicyEvalTrace:
    rule_id: str
    score: float
    policy_schema: Optional[str] = None
    entity_type: Optional[str] = None
    matched_when: Optional[Dict[str, Any]] = None


class PolicyEngine:
    """
    ✅ v1 policy engine（threshold + rules）
    ✅ extends: _base.yaml を解決してから評価
    ✅ evaluate input を明確化: context={"score": float, ...}
    """

    def __init__(self, package: str = "atlaskernel.policies") -> None:
        self.package = package
        self._cache: Dict[str, Dict[str, Any]] = {}

    # ---------------------------------------------------------
    # Load (with extends)
    # ---------------------------------------------------------
    def load(self, entity_type: str) -> Dict[str, Any]:
        """
        entity_type: "brand" | "condition" | "color" | "document_term" ...
        loads atlaskernel/policies/{entity_type}.yaml and resolves extends chain.
        """
        if entity_type in self._cache:
            return self._cache[entity_type]

        resource = f"{entity_type}.yaml"
        policy = self._load_yaml(resource)

        # resolve extends (ex: "_base.yaml")
        extends = policy.get("extends")
        if extends:
            base = self._load_yaml(extends)
            # base itself could have extends (rare) - resolve recursively
            base_ext = base.get("extends")
            if base_ext:
                base2 = self._load_yaml(base_ext)
                base = _deep_merge(base2, base)
            policy = _deep_merge(base, policy)

        self._cache[entity_type] = policy
        return policy

    def _load_yaml(self, resource_name: str) -> Dict[str, Any]:
        try:
            with resources.files(self.package).joinpath(resource_name).open("r", encoding="utf-8") as f:
                return yaml.safe_load(f) or {}
        except FileNotFoundError:
            raise FileNotFoundError(f"Policy not found: {self.package.replace('.', '/')}/{resource_name}")

    # ---------------------------------------------------------
    # Evaluate
    # ---------------------------------------------------------
    def evaluate(self, policy: Dict[str, Any], context: Dict[str, Any]) -> Tuple[str, str, Dict[str, Any]]:
        """
        returns: (decision, reason, trace_dict)

        decision:
          - "auto_accept"
          - "needs_review"
          - "rejected"
        """
        if "score" not in context:
            raise ValueError("PolicyEngine.evaluate requires context['score']")

        score = float(context["score"] or 0.0)

        # 1) rules first
        for rule in policy.get("rules", []) or []:
            when = rule.get("when", {}) or {}
            if self._match(when, context):
                trace = PolicyEvalTrace(
                    rule_id=str(rule.get("id") or "rule"),
                    score=score,
                    policy_schema=policy.get("schema"),
                    entity_type=policy.get("entity_type"),
                    matched_when=when,
                )
                return (
                    str(rule.get("decision") or "rejected"),
                    str(rule.get("reason") or "rule_match"),
                    trace.__dict__,
                )

        # 2) threshold fallback (overrides -> defaults)
        actions = (
            (policy.get("overrides") or {}).get("actions")
            or (policy.get("defaults") or {}).get("actions")
            or {}
        )

        def _min_score(action_key: str, default: float) -> float:
            a = actions.get(action_key) or {}
            return float(a.get("min_score", default))

        auto_min = _min_score("auto_accept", 0.85)
        review_min = _min_score("needs_review", 0.35)

        if score >= auto_min:
            return "auto_accept", "threshold", {
                "rule_id": "threshold_auto_accept",
                "score": score,
                "policy_schema": policy.get("schema"),
                "entity_type": policy.get("entity_type"),
            }

        if score >= review_min:
            return "needs_review", "threshold", {
                "rule_id": "threshold_needs_review",
                "score": score,
                "policy_schema": policy.get("schema"),
                "entity_type": policy.get("entity_type"),
            }

        return "rejected", "threshold", {
            "rule_id": "threshold_rejected",
            "score": score,
            "policy_schema": policy.get("schema"),
            "entity_type": policy.get("entity_type"),
        }

    def _match(self, when: Dict[str, Any], context: Dict[str, Any]) -> bool:
        score = float(context.get("score") or 0.0)

        if "score_gte" in when and score < float(when["score_gte"]):
            return False
        if "score_lt" in when and score >= float(when["score_lt"]):
            return False

        # optional flags
        if "has_alias" in when:
            if bool(context.get("has_alias")) != bool(when["has_alias"]):
                return False

        return True