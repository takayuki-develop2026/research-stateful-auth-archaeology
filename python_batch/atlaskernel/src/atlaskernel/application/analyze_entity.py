from __future__ import annotations

import os
import re
import sys
import logging
from typing import List, Dict, Any, Tuple, Optional

from atlaskernel.application.pipelines.brand_pipeline import analyze_brand
from atlaskernel.application.pipelines.condition_pipeline import analyze_condition
from atlaskernel.application.pipelines.color_pipeline import analyze_color

from atlaskernel.services.policy_engine import PolicyEngine
from atlaskernel.services.policy_engine_v2 import PolicyEngineV2
from atlaskernel.services.context_builder import ContextBuilder

from atlaskernel.domain.request import AnalysisRequest
from atlaskernel.domain.context import Context

logger = logging.getLogger("atlaskernel.analyze_entity")


def _d(msg: str) -> None:
    # docker compose logs で必ず見える（stderr）
    print(msg, file=sys.stderr, flush=True)


def _normalize_text(s: str) -> str:
    if not isinstance(s, str):
        return ""
    s = s.strip()

    try:
        import unicodedata
        s = unicodedata.normalize("NFKC", s)
    except Exception:
        pass

    s = re.sub(r"\s+", " ", s).strip()
    s = s.replace("／", "/").replace("－", "-").replace("〜", "~")
    return s


def _strip_all_spaces(s: Optional[str]) -> Optional[str]:
    """
    brand_text / condition_text / color_text は
    "あっぷ る" のような事故を潰すために「全空白除去」が安全。
    """
    if not s:
        return None
    if not isinstance(s, str):
        s = str(s)
    s = s.strip()
    if not s:
        return None
    s2 = re.sub(r"\s+", "", s)
    return s2.strip() or None


def _extract_name_conf_tokens(r: Any) -> Tuple[Optional[str], float, List[str]]:
    """
    pipelineの返却が dict / object どちらでも拾って
    (name, confidence, tokens) を返す
    """
    if r is None:
        return (None, 0.0, [])

    # object case
    if not isinstance(r, dict):
        name = (
            getattr(r, "canonical_value", None)
            or getattr(r, "canonical_name", None)
            or getattr(r, "name", None)
            or getattr(r, "value", None)
        )
        name = name.strip() if isinstance(name, str) and name.strip() else None

        conf = getattr(r, "confidence", 0.0)
        try:
            conf = float(conf or 0.0)
        except Exception:
            conf = 0.0

        tokens: List[str] = []
        ext = getattr(r, "extensions", None)
        if isinstance(ext, dict):
            for k in ("tokens", "matches", "hits"):
                v = ext.get(k)
                if isinstance(v, list):
                    tokens = [str(x).strip() for x in v if str(x).strip()]
                    break

        return (name, conf, tokens)

    # dict case
    name: Optional[str] = None
    candidates = [
        r.get("name"),
        r.get("canonical_name"),
        r.get("canonical_value"),
        r.get("value"),
        r.get("label"),
        (r.get("entity") or {}).get("name"),
        (r.get("entity") or {}).get("canonical_name"),
        (r.get("result") or {}).get("name"),
        (r.get("result") or {}).get("canonical_name"),
        (r.get("result") or {}).get("canonical_value"),
        (r.get("result") or {}).get("value"),
    ]
    for v in candidates:
        if isinstance(v, str) and v.strip():
            name = v.strip()
            break

    conf = 0.0
    for k in ("confidence", "score", "prob", "likelihood", "overall_confidence", "max_confidence"):
        v = r.get(k)
        if isinstance(v, (int, float)):
            conf = float(v)
            break

    if conf == 0.0 and isinstance(r.get("result"), dict):
        for k in ("confidence", "score", "prob", "likelihood", "overall_confidence", "max_confidence"):
            v = r["result"].get(k)
            if isinstance(v, (int, float)):
                conf = float(v)
                break

    tokens: List[str] = []
    for k in ("tokens", "classified_tokens", "matches", "hits"):
        v = r.get(k)
        if isinstance(v, list):
            tokens = [str(x).strip() for x in v if str(x).strip()]
            break
        if isinstance(v, dict):
            tmp: List[str] = []
            for _, arr in v.items():
                if isinstance(arr, list):
                    tmp.extend([str(x).strip() for x in arr if str(x).strip()])
            if tmp:
                tokens = tmp
                break

    if not tokens and isinstance(r.get("result"), dict):
        v = r["result"].get("tokens")
        if isinstance(v, list):
            tokens = [str(x).strip() for x in v if str(x).strip()]

    return (name, conf, tokens)


def _keys_of(x: Any) -> List[str]:
    if isinstance(x, dict):
        return list(x.keys())
    return [
        k
        for k in ("entity_type", "raw_value", "canonical_value", "confidence", "decision", "rule_id")
        if hasattr(x, k)
    ]


def _tokenize(raw_text: str) -> List[str]:
    """
    ✅ 順不同耐性の核心
    - split済み raw_text を空白tokenize
    - 1文字でも CJK を落とさない（"青" "赤" 等）
    """
    toks = [t.strip() for t in raw_text.split(" ") if t.strip()]

    def keep(t: str) -> bool:
        if len(t) >= 2:
            return True
        return any(ord(ch) > 127 for ch in t)  # 1文字CJKは採用

    return [t for t in toks if keep(t)]


# =========================================================
# ✅ hint tokens（スペース無し結合を救う）
# =========================================================
CONDITION_HINTS = ["新品", "しんぴん", "未使用", "美品", "中古", "ジャンク"]
COLOR_HINTS = ["赤", "あか", "青", "あお", "黒", "くろ", "白", "しろ", "銀", "グレー", "緑", "黄"]


def _hint_tokens(raw_text: str, hints: List[str]) -> List[str]:
    s = _strip_all_spaces(raw_text) or ""
    out: List[str] = []
    for h in hints:
        if h and (h in s):
            out.append(h)
    return out


# =========================================================
# ✅ BRAND_HINTS を brands_v1.txt からロード（初回のみキャッシュ）
# =========================================================
_BRAND_HINT_CACHE: Optional[List[str]] = None


def _load_brand_hints_from_file() -> List[str]:
    # docker 側でマウントするパスに合わせる
    # 例: /app/assets/brands_v1.txt を推奨（ATLAS_BRANDS_HINT_PATH で上書き可）
    path = os.getenv("ATLAS_BRANDS_HINT_PATH", "/app/src/atlaskernel/assets/brands_v1.txt")
    try:
        hints: List[str] = []
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                s = line.strip()
                if not s or s.startswith("#"):
                    continue
                hints.append(s)
        _d(f"[AK] brands hint loaded path={path} count={len(hints)}")
        return hints
    except Exception as e:
        _d(f"[AK] brands hint load failed path={path} err={e}")
        return []


def _get_brand_hints() -> List[str]:
    global _BRAND_HINT_CACHE
    if _BRAND_HINT_CACHE is None:
        _BRAND_HINT_CACHE = _load_brand_hints_from_file()
    return _BRAND_HINT_CACHE


# =========================================================
# ✅ brand_text force を安全化（誤爆防止）
# =========================================================
def _looks_dirty_brand(s: str) -> bool:
    if not s:
        return True
    # 長すぎるのは説明文率が高い（暫定）
    if len(s) > 10:
        return True
    dirty = ["新品", "しんぴん", "美品", "中古", "赤", "あか", "青", "あお", "黒", "白"]
    return any(d in s for d in dirty)


def _run_and_pick_best(
    *,
    entity_type: str,
    analyze_fn,
    tokens: List[str],
    raw_text: str,
    policy,
    ctx: Context,
    req_ctx: Dict[str, Any],
    known_assets_ref: str,
    min_conf: float = 0.80,  # 誤爆止め
) -> Tuple[Optional[Any], bool, Optional[str], List[Dict[str, Any]]]:
    """
    returns:
      (best_result, used_fallback_fulltext, best_tok, tried_list)
    """
    best_r: Optional[Any] = None
    best_conf: float = -1.0
    best_tok: Optional[str] = None
    tried: List[Dict[str, Any]] = []

    for tok in tokens:
        try:
            r = analyze_fn(
                AnalysisRequest(
                    entity_type=entity_type,
                    raw_value=str(tok),
                    known_assets_ref=known_assets_ref,
                    context=req_ctx,
                ),
                policy,
                ctx,
            )
            name, conf, _ = _extract_name_conf_tokens(r)
            conf_f = float(conf or 0.0)

            tried.append({"tok": tok, "name": name, "conf": conf_f})

            if not name or conf_f < min_conf:
                continue

            if conf_f > best_conf:
                best_conf = conf_f
                best_r = r
                best_tok = tok

        except Exception as e:
            tried.append({"tok": tok, "name": None, "conf": 0.0, "err": str(e)})
            _d(f"[AK] token analyze failed entity={entity_type} tok={tok} err={e}")

    if best_r is not None:
        return (best_r, False, best_tok, tried)

    # fallback: fulltext
    try:
        r = analyze_fn(
            AnalysisRequest(
                entity_type=entity_type,
                raw_value=str(raw_text),
                known_assets_ref=known_assets_ref,
                context=req_ctx,
            ),
            policy,
            ctx,
        )
        name, conf, _ = _extract_name_conf_tokens(r)
        conf_f = float(conf or 0.0)
        tried.append({"tok": "__fulltext__", "name": name, "conf": conf_f})

        if not name or conf_f < min_conf:
            return (None, True, "__fulltext__", tried)

        return (r, True, "__fulltext__", tried)

    except Exception as e:
        tried.append({"tok": "__fulltext__", "name": None, "conf": 0.0, "err": str(e)})
        _d(f"[AK] fulltext analyze failed entity={entity_type} err={e}")
        return (None, True, "__fulltext__", tried)


def analyze(request: AnalysisRequest) -> Dict[str, Any]:
    """
    ✅ Laravel互換のフラット構造
    ✅ token best-of（順不同耐性）
    ✅ fulltext fallback は最後
    ✅ min_conf 未満は None
    ✅ human hints は force 採用（ただし brand_text はガード付き）
    ✅ スペース無し結合は hint tokens で救う
    ✅ BRAND_HINTS は brands_v1.txt からロード（初回キャッシュ）
    """
    engine_name = os.getenv("ATLAS_POLICY_ENGINE", "v1")
    policy = PolicyEngineV2() if engine_name == "v2" else PolicyEngine()
    ctx: Context = ContextBuilder().build(base={}, multimodal=None)

    req_ctx = getattr(request, "context", {}) or {}
    raw_text = _normalize_text(getattr(request, "raw_value", "") or "")

    # ---- human hints ----
    brand_text = req_ctx.get("brand_text") or req_ctx.get("attributes_text")
    condition_text = req_ctx.get("condition_text")
    color_text = req_ctx.get("color_text")

    brand_text = _strip_all_spaces(_normalize_text(str(brand_text))) if brand_text else None
    condition_text = _strip_all_spaces(_normalize_text(str(condition_text))) if condition_text else None
    color_text = _strip_all_spaces(_normalize_text(str(color_text))) if color_text else None

    # ---- token build ----
    raw_tokens = _tokenize(raw_text)

    # condition/color hint（既存）
    condition_tokens = list(dict.fromkeys(_hint_tokens(raw_text, CONDITION_HINTS) + raw_tokens))
    color_tokens = list(dict.fromkeys(_hint_tokens(raw_text, COLOR_HINTS) + raw_tokens))

    # ✅ brand hint（brands_v1.txt）
    brand_hints = _get_brand_hints()
    brand_hint_tokens = _hint_tokens(raw_text, brand_hints)

    # # （任意）ひらがな「あっぷる」→ カタカナ「アップル」救済（辞書がカタカナのみの場合）
    # if "あっぷる" in brand_hint_tokens and "アップル" not in brand_hint_tokens:
    #     brand_hint_tokens.append("アップル")

    brand_tokens = list(dict.fromkeys(brand_hint_tokens + raw_tokens))

    # brand_text は混ぜる（force はガード付きのまま）
    if brand_text and brand_text != raw_text:
        bt = _strip_all_spaces(brand_text)
        if bt:
            brand_tokens = list(dict.fromkeys(brand_tokens + _tokenize(bt)))

    # ---- thresholds ----
    brand_min = 0.80
    cond_min = 0.85
    color_min = 0.85

    # ---- analyze ----
    brand_r, brand_fallback, brand_best_tok, brand_tried = _run_and_pick_best(
        entity_type="brand",
        analyze_fn=analyze_brand,
        tokens=brand_tokens,
        raw_text=raw_text,
        policy=policy,
        ctx=ctx,
        req_ctx=req_ctx,
        known_assets_ref="brands_v1",
        min_conf=brand_min,
    )

    cond_r, condition_fallback, cond_best_tok, cond_tried = _run_and_pick_best(
        entity_type="condition",
        analyze_fn=analyze_condition,
        tokens=condition_tokens,
        raw_text=raw_text,
        policy=policy,
        ctx=ctx,
        req_ctx=req_ctx,
        known_assets_ref="conditions_v1",
        min_conf=cond_min,
    )

    color_r, color_fallback, color_best_tok, color_tried = _run_and_pick_best(
        entity_type="color",
        analyze_fn=analyze_color,
        tokens=color_tokens,
        raw_text=raw_text,
        policy=policy,
        ctx=ctx,
        req_ctx=req_ctx,
        known_assets_ref="colors_v1",
        min_conf=color_min,
    )

    # ---- diagnostics ----
    _d(f"[AK] raw_text={raw_text} brand_text={brand_text} condition_text={condition_text} color_text={color_text}")
    _d(f"[AK] raw_tokens={raw_tokens}")
    _d(f"[AK] brand_tokens={brand_tokens}")
    _d(f"[AK] condition_tokens={condition_tokens}")
    _d(f"[AK] color_tokens={color_tokens}")

    _d(f"[AK] tried brand={brand_tried}")
    _d(f"[AK] tried condition={cond_tried}")
    _d(f"[AK] tried color={color_tried}")

    _d(f"[AK] picked brand keys={_keys_of(brand_r)} fallback={brand_fallback} best_tok={brand_best_tok}")
    _d(f"[AK] picked cond  keys={_keys_of(cond_r)} fallback={condition_fallback} best_tok={cond_best_tok}")
    _d(f"[AK] picked color keys={_keys_of(color_r)} fallback={color_fallback} best_tok={color_best_tok}")

    # ---- extract ----
    brand_name, brand_conf, brand_tokens_from_pipe = _extract_name_conf_tokens(brand_r)
    cond_name, cond_conf, cond_tokens_from_pipe = _extract_name_conf_tokens(cond_r)
    color_name, color_conf, color_tokens_from_pipe = _extract_name_conf_tokens(color_r)

    def _safe_float(x: Any) -> float:
        try:
            return float(x or 0.0)
        except Exception:
            return 0.0

    confidence_map = {
        "brand": _safe_float(brand_conf),
        "condition": _safe_float(cond_conf),
        "color": _safe_float(color_conf),
    }
    overall_confidence = max(confidence_map.values()) if confidence_map else 0.0

    def _merge_tokens(pipe_tokens: List[str], best_tok: Optional[str]) -> List[str]:
        out: List[str] = []
        for t in (pipe_tokens or []):
            tt = str(t).strip()
            if tt and tt not in out:
                out.append(tt)
        if best_tok:
            bt = str(best_tok).strip()
            if bt and bt not in out:
                out.append(bt)
        return out

    tokens_out = {
        "brand": _merge_tokens(brand_tokens_from_pipe, brand_best_tok),
        "condition": _merge_tokens(cond_tokens_from_pipe, cond_best_tok),
        "color": _merge_tokens(color_tokens_from_pipe, color_best_tok),
    }

    # =========================================================
    # ✅ FORCE ADOPT（人が入力したものは最強根拠）
    # - brand_text は「汚れてなければ」force
    # - condition/color はそのまま force（入ってたら）
    # =========================================================

    # brand force（ガード付き）
    if brand_text:
        bt_force = _strip_all_spaces(brand_text)
        if bt_force and (not _looks_dirty_brand(bt_force)):
            brand_name = bt_force
            confidence_map["brand"] = max(confidence_map.get("brand", 0.0), 0.95)
            overall_confidence = max(overall_confidence, confidence_map["brand"])
            tokens_out["brand"] = [t for t in (tokens_out.get("brand") or []) if t != "__fulltext__"]
            tokens_out["brand"] = list(dict.fromkeys([bt_force] + (tokens_out.get("brand") or [])))

    # condition force
    ct_force = None
    if condition_text:
        ct_force = _strip_all_spaces(condition_text)
    if ct_force:
        cond_name = ct_force
        confidence_map["condition"] = max(confidence_map.get("condition", 0.0), 0.95)
        overall_confidence = max(overall_confidence, confidence_map["condition"])
        tokens_out["condition"] = [t for t in (tokens_out.get("condition") or []) if t != "__fulltext__"]
        tokens_out["condition"] = list(dict.fromkeys([ct_force] + (tokens_out.get("condition") or [])))

    # color force（必要なら。colorカラム無しでも ctx に入ってこなければ無視）
    cl_force = None
    if color_text:
        cl_force = _strip_all_spaces(color_text)
    if cl_force:
        color_name = cl_force
        confidence_map["color"] = max(confidence_map.get("color", 0.0), 0.95)
        overall_confidence = max(overall_confidence, confidence_map["color"])
        tokens_out["color"] = [t for t in (tokens_out.get("color") or []) if t != "__fulltext__"]
        tokens_out["color"] = list(dict.fromkeys([cl_force] + (tokens_out.get("color") or [])))

    return {
        "brand": {"name": brand_name, "source": "ai_provisional"},
        "condition": {"name": cond_name, "source": "ai_provisional"},
        "color": {"name": color_name, "source": "ai_provisional"},
        "tokens": tokens_out,
        "confidence_map": confidence_map,
        "overall_confidence": overall_confidence,
        "source": "ai_provisional",
    }