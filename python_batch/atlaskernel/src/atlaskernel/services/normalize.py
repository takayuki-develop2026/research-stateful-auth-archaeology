from __future__ import annotations

import os
import re
import unicodedata
from typing import Dict, List, Optional

# ----------------------------------------------------------
# Kana utilities
# ----------------------------------------------------------
def hira_to_kata(s: str) -> str:
    out = []
    for ch in s:
        code = ord(ch)
        if 0x3041 <= code <= 0x3096:
            out.append(chr(code + 0x60))
        else:
            out.append(ch)
    return "".join(out)

_JP_KEEP = r"\u3040-\u30FF\u4E00-\u9FFF"  # hiragana/katakana/kanji
_RE_NON_KEEP = re.compile(rf"[^\w\s{_JP_KEEP}]")
_RE_SPACE = re.compile(r"\s+")


def normalize_key(text: str) -> str:
    """
    DB-grade normalize for entity keys:
    - NFKC normalize
    - lower
    - keep: word chars + spaces + Japanese ranges
    - punctuation -> space
    - collapse spaces
    - hiragana -> katakana
    """
    if not text:
        return ""

    t = unicodedata.normalize("NFKC", str(text))
    t = t.strip().lower()

    t = _RE_NON_KEEP.sub(" ", t)
    t = _RE_SPACE.sub(" ", t).strip()

    t = hira_to_kata(t)
    t = _RE_SPACE.sub(" ", t).strip()
    return t


def normalize(text: str) -> str:
    """
    ✅ 基本は normalize_key と同義（alias置換はしない）。
    互換のため、ATLAS_NORMALIZE_APPLY_BRAND_ALIAS=1 のときのみ
    “内蔵 alias map” を適用できるよう残す（デフォルトOFF推奨）。
    """
    nk = normalize_key(text)
    if not nk:
        return ""

    if os.getenv("ATLAS_NORMALIZE_APPLY_BRAND_ALIAS", "0") == "1":
        # legacy compatibility (ONLY if you really need it)
        # Keep this minimal & stable; primary SoT should be assets/*.tsv in pipelines.
        BRAND_ALIASES_RAW = {
            "rolax": "rolex",
            "ロレックス": "rolex",
            "ro lex": "rolex",
            "アップル": "apple",
            "あっぷる": "apple",
            "apple": "apple",
        }
        m: Dict[str, str] = {normalize_key(k): v for k, v in BRAND_ALIASES_RAW.items() if normalize_key(k)}
        return m.get(nk, nk)

    return nk


def normalize_text(text: str) -> str:
    """
    Generic text normalize (kept for compatibility).
    """
    if not text:
        return ""
    t = unicodedata.normalize("NFKC", str(text))
    t = t.lower().strip()
    t = _RE_SPACE.sub(" ", t)
    return t