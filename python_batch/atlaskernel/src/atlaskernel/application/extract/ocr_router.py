from __future__ import annotations

import base64
import hashlib
import io
import time
from dataclasses import dataclass
from typing import Any, Optional, Literal

from pdfminer.high_level import extract_text as pdf_extract_text

Engine = Literal["auto", "tesseract", "paddleocr", "doctr", "textract", "document_ai"]
Mode = Literal["auto", "force_ocr"]


@dataclass(frozen=True)
class Budget:
    max_ms: int = 3000
    max_cost_usd: float = 0.0


@dataclass(frozen=True)
class OcrOptions:
    engine: Engine = "auto"
    mode: Mode = "auto"
    lang: str = "jpn"
    dpi: int = 200
    max_pages: int = 3
    min_length_for_no_ocr: int = 200
    min_confidence: float = 70.0
    budget: Budget = Budget()


@dataclass(frozen=True)
class ExtractResult:
    text: str
    meta: dict[str, Any]


class OcrRouter:
    def extract_pdf_text(self, pdf_bytes: bytes, source_url: Optional[str], opt: OcrOptions) -> ExtractResult:
        t0 = time.perf_counter()
        text = ""
        err = None

        try:
            text = pdf_extract_text(io.BytesIO(pdf_bytes)) or ""
        except Exception as e:
            err = str(e)

        text = self._normalize(text)
        elapsed_ms = int((time.perf_counter() - t0) * 1000)

        meta: dict[str, Any] = {
            "method": "pdfminer.six",
            "length": len(text),
            "source_url": source_url,
            "elapsed_ms": elapsed_ms,
            "cost_usd": 0.0,
            "ocr_recommended": len(text) < opt.min_length_for_no_ocr,
            "min_length_for_no_ocr": opt.min_length_for_no_ocr,
        }
        if err:
            meta["pdf_text_error"] = err

        return ExtractResult(text=text, meta=meta)

    def extract_pdf_ocr(self, pdf_bytes: bytes, source_url: Optional[str], opt: OcrOptions, engine_selected: str) -> ExtractResult:
        if engine_selected == "paddleocr":
            return self._extract_pdf_ocr_paddleocr(pdf_bytes, source_url, opt, engine_selected)
        return self._extract_pdf_ocr_tesseract(pdf_bytes, source_url, opt, engine_selected)

    def extract_image_ocr(self, image_bytes: bytes, source_url: Optional[str], opt: OcrOptions) -> ExtractResult:
        engine_selected = self._resolve_engine(opt.engine)
        if engine_selected == "paddleocr":
            return self._extract_image_ocr_paddleocr(image_bytes, source_url, opt, engine_selected)
        return self._extract_image_ocr_tesseract(image_bytes, source_url, opt, engine_selected)

    def _extract_pdf_ocr_tesseract(self, pdf_bytes: bytes, source_url: Optional[str], opt: OcrOptions, engine_selected: str) -> ExtractResult:
        t0 = time.perf_counter()

        try:
            from pdf2image import convert_from_bytes
            images = convert_from_bytes(pdf_bytes, dpi=opt.dpi)
        except Exception as e:
            return ExtractResult(
                text="",
                meta={
                    "method": "tesseract",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 0,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "ocr_error": f"pdf->image failed: {e}",
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        if not images:
            return ExtractResult(
                text="",
                meta={
                    "method": "tesseract",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 0,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        images = images[: max(1, opt.max_pages)]

        try:
            import pytesseract
        except Exception as e:
            return ExtractResult(
                text="",
                meta={
                    "method": "tesseract",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": len(images),
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "ocr_error": f"pytesseract import failed: {e}",
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        texts: list[str] = []
        confs: list[float] = []
        blocks: list[dict[str, Any]] = []
        seq = 1

        for page_index, img in enumerate(images, start=1):
            page_text, page_confs, page_blocks, seq = self._run_tesseract_on_image(
                img=img,
                lang=opt.lang,
                page_index=page_index,
                seq_start=seq,
            )
            texts.append(page_text)
            confs.extend(page_confs)
            blocks.extend(page_blocks)

        return self._build_ocr_result(
            method="tesseract",
            engine_selected=engine_selected,
            lang=opt.lang,
            dpi=opt.dpi,
            pages=len(images),
            text=self._normalize("\n\n".join(texts)),
            confs=confs,
            blocks=blocks,
            source_url=source_url,
            started_at=t0,
        )

    def _extract_pdf_ocr_paddleocr(self, pdf_bytes: bytes, source_url: Optional[str], opt: OcrOptions, engine_selected: str) -> ExtractResult:
        t0 = time.perf_counter()

        try:
            from pdf2image import convert_from_bytes
            images = convert_from_bytes(pdf_bytes, dpi=opt.dpi)
        except Exception as e:
            return ExtractResult(
                text="",
                meta={
                    "method": "paddleocr",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 0,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "ocr_error": f"pdf->image failed: {e}",
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        if not images:
            return ExtractResult(
                text="",
                meta={
                    "method": "paddleocr",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 0,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        images = images[: max(1, opt.max_pages)]

        texts: list[str] = []
        confs: list[float] = []
        blocks: list[dict[str, Any]] = []
        seq = 1

        for page_index, img in enumerate(images, start=1):
            page_text, page_confs, page_blocks, seq = self._run_paddleocr_on_image(
                img=img,
                lang=opt.lang,
                page_index=page_index,
                seq_start=seq,
            )
            texts.append(page_text)
            confs.extend(page_confs)
            blocks.extend(page_blocks)

        return self._build_ocr_result(
            method="paddleocr",
            engine_selected=engine_selected,
            lang=self._to_paddle_lang(opt.lang),
            dpi=opt.dpi,
            pages=len(images),
            text=self._normalize("\n\n".join(texts)),
            confs=confs,
            blocks=blocks,
            source_url=source_url,
            started_at=t0,
        )

    def _extract_image_ocr_tesseract(self, image_bytes: bytes, source_url: Optional[str], opt: OcrOptions, engine_selected: str) -> ExtractResult:
        t0 = time.perf_counter()

        try:
            from PIL import Image
            img = Image.open(io.BytesIO(image_bytes))
        except Exception as e:
            return ExtractResult(
                text="",
                meta={
                    "method": "tesseract",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 1,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "ocr_error": f"image decode failed: {e}",
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        text, confs, blocks, _ = self._run_tesseract_on_image(
            img=img,
            lang=opt.lang,
            page_index=1,
            seq_start=1,
        )

        return self._build_ocr_result(
            method="tesseract",
            engine_selected=engine_selected,
            lang=opt.lang,
            dpi=opt.dpi,
            pages=1,
            text=self._normalize(text),
            confs=confs,
            blocks=blocks,
            source_url=source_url,
            started_at=t0,
        )

    def _extract_image_ocr_paddleocr(self, image_bytes: bytes, source_url: Optional[str], opt: OcrOptions, engine_selected: str) -> ExtractResult:
        t0 = time.perf_counter()

        try:
            from PIL import Image
            img = Image.open(io.BytesIO(image_bytes))
        except Exception as e:
            return ExtractResult(
                text="",
                meta={
                    "method": "paddleocr",
                    "engine_selected": engine_selected,
                    "lang": opt.lang,
                    "dpi": opt.dpi,
                    "pages": 1,
                    "length": 0,
                    "source_url": source_url,
                    "elapsed_ms": int((time.perf_counter() - t0) * 1000),
                    "cost_usd": 0.0,
                    "ocr_error": f"image decode failed: {e}",
                    "avg_confidence": None,
                    "ocr_blocks": [],
                },
            )

        text, confs, blocks, _ = self._run_paddleocr_on_image(
            img=img,
            lang=opt.lang,
            page_index=1,
            seq_start=1,
        )

        return self._build_ocr_result(
            method="paddleocr",
            engine_selected=engine_selected,
            lang=self._to_paddle_lang(opt.lang),
            dpi=opt.dpi,
            pages=1,
            text=self._normalize(text),
            confs=confs,
            blocks=blocks,
            source_url=source_url,
            started_at=t0,
        )

    def _run_tesseract_on_image(self, img: Any, lang: str, page_index: int, seq_start: int):
        try:
            import pytesseract
        except Exception as e:
            raise RuntimeError(f"pytesseract import failed: {e}")

        texts: list[str] = []
        confs: list[float] = []
        blocks: list[dict[str, Any]] = []
        seq = seq_start

        page_text = pytesseract.image_to_string(img, lang=lang) or ""
        texts.append(page_text)

        try:
            data = pytesseract.image_to_data(img, lang=lang, output_type=pytesseract.Output.DICT)
            n = len(data.get("text", []) or [])
            for i in range(n):
                txt = (data.get("text", [])[i] or "").strip()
                raw_conf = (data.get("conf", [])[i] if i < len(data.get("conf", [])) else None)

                score: Optional[float] = None
                try:
                    if raw_conf is not None:
                        c = float(raw_conf)
                        if c >= 0:
                            confs.append(c)
                            score = c
                except Exception:
                    score = None

                if txt:
                    left = self._safe_int(data.get("left", []), i)
                    top = self._safe_int(data.get("top", []), i)
                    width = self._safe_int(data.get("width", []), i)
                    height = self._safe_int(data.get("height", []), i)
                    box = [left, top, left + width, top + height]

                    blocks.append(
                        {
                            "text": txt,
                            "score": score,
                            "box": box,
                            "seq": seq,
                            "role": "ocr_line",
                            "page": page_index,
                        }
                    )
                    seq += 1
        except Exception:
            pass

        return "\n".join(texts), confs, blocks, seq

    def _run_paddleocr_on_image(self, img: Any, lang: str, page_index: int, seq_start: int):
        try:
            from paddleocr import PaddleOCR
        except Exception as e:
            raise RuntimeError(f"paddleocr import failed: {e}")

        try:
            import numpy as np
        except Exception as e:
            raise RuntimeError(f"numpy import failed: {e}")

        paddle_lang = self._to_paddle_lang(lang)
        try:
            ocr = PaddleOCR(
                use_angle_cls=True,
                lang=paddle_lang,
                show_log=False,
            )
        except Exception as e:
            raise RuntimeError(f"paddleocr init failed: {e}")

        np_img = np.array(img)
        result = ocr.ocr(np_img, cls=True)

        page_lines: list[str] = []
        confs: list[float] = []
        blocks: list[dict[str, Any]] = []
        seq = seq_start

        for line in self._normalize_paddle_result(result):
            txt = (line.get("text") or "").strip()
            score = line.get("score")
            box = line.get("box") or []

            if txt:
                page_lines.append(txt)

            if isinstance(score, (int, float)):
                confs.append(float(score))

            if txt:
                blocks.append(
                    {
                        "text": txt,
                        "score": float(score) if isinstance(score, (int, float)) else None,
                        "box": box,
                        "seq": seq,
                        "role": "ocr_line",
                        "page": page_index,
                    }
                )
                seq += 1

        return "\n".join(page_lines), confs, blocks, seq

    def _build_ocr_result(
        self,
        *,
        method: str,
        engine_selected: str,
        lang: str,
        dpi: int,
        pages: int,
        text: str,
        confs: list[float],
        blocks: list[dict[str, Any]],
        source_url: Optional[str],
        started_at: float,
    ) -> ExtractResult:
        avg_conf = (sum(confs) / len(confs)) if confs else None
        elapsed_ms = int((time.perf_counter() - started_at) * 1000)

        meta: dict[str, Any] = {
            "method": method,
            "engine_selected": engine_selected,
            "lang": lang,
            "dpi": dpi,
            "pages": pages,
            "length": len(text),
            "avg_confidence": avg_conf,
            "source_url": source_url,
            "elapsed_ms": elapsed_ms,
            "cost_usd": 0.0,
            "ocr_recommended": False,
            "ocr_blocks": blocks,
        }

        return ExtractResult(text=text, meta=meta)

    def route(self, pdf_bytes: bytes, source_url: Optional[str], opt: OcrOptions) -> ExtractResult:
        engine_requested = opt.engine
        engine_selected = self._resolve_engine(engine_requested)

        if opt.mode == "force_ocr":
            r = self.extract_pdf_ocr(pdf_bytes, source_url, opt, engine_selected)
            meta = dict(r.meta)
            meta.update(self._audit_fixed(opt, engine_requested, engine_selected))
            meta["pipeline"] = "pdf_ocr_only"
            meta["fallback_chain"] = ["pdf_ocr"]
            return self._finalize(meta, r.text)

        t = self.extract_pdf_text(pdf_bytes, source_url, opt)
        meta_t = dict(t.meta)
        meta_t.update(self._audit_fixed(opt, engine_requested, engine_selected))

        if meta_t.get("ocr_recommended") is True:
            o = self.extract_pdf_ocr(pdf_bytes, source_url, opt, engine_selected)

            if self._should_fallback_to_tesseract(engine_requested, engine_selected, o):
                o2 = self.extract_pdf_ocr(pdf_bytes, source_url, opt, "tesseract")
                meta = dict(meta_t)
                meta.update(o2.meta)
                meta["pipeline"] = "pdf_text_then_ocr"
                meta["fallback_chain"] = ["pdf_text", "pdf_ocr:paddleocr", "pdf_ocr:tesseract"]
                text = o2.text.strip() and o2.text or t.text
                if not o2.text.strip():
                    meta["ocr_error"] = meta.get("ocr_error") or "ocr returned empty text after fallback"
                return self._finalize(meta, text)

            meta = dict(meta_t)
            meta.update(o.meta)
            meta["pipeline"] = "pdf_text_then_ocr"
            meta["fallback_chain"] = ["pdf_text", f"pdf_ocr:{engine_selected}"]

            text = o.text.strip() and o.text or t.text
            if not o.text.strip():
                meta["ocr_error"] = meta.get("ocr_error") or "ocr returned empty text"

            return self._finalize(meta, text)

        meta_t["pipeline"] = "pdf_text_only"
        meta_t["fallback_chain"] = ["pdf_text"]
        return self._finalize(meta_t, t.text)

    def _resolve_engine(self, engine_requested: str) -> str:
        if engine_requested == "paddleocr":
            return "paddleocr"
        if engine_requested == "tesseract":
            return "tesseract"
        return "paddleocr"

    def _should_fallback_to_tesseract(self, engine_requested: str, engine_selected: str, result: ExtractResult) -> bool:
        if engine_requested != "auto":
            return False
        if engine_selected != "paddleocr":
            return False
        txt = (result.text or "").strip()
        if txt:
            return False
        return True

    def _audit_fixed(self, opt: OcrOptions, engine_requested: str, engine_selected: str) -> dict[str, Any]:
        return {
            "engine_requested": engine_requested,
            "engine_selected": engine_selected,
            "mode": opt.mode,
            "lang": opt.lang,
            "min_confidence": opt.min_confidence,
            "budget": {"max_ms": opt.budget.max_ms, "max_cost_usd": opt.budget.max_cost_usd},
        }

    def _finalize(self, meta: dict[str, Any], text: str) -> ExtractResult:
        elapsed_ms = meta.get("elapsed_ms")
        meta["elapsed_ms_total"] = elapsed_ms if isinstance(elapsed_ms, int) else None

        max_ms = int(meta.get("budget", {}).get("max_ms") or 3000)
        budget_exceeded = isinstance(elapsed_ms, int) and elapsed_ms > max_ms
        meta["budget_exceeded"] = budget_exceeded

        avg_conf = meta.get("avg_confidence")
        min_conf = float(meta.get("min_confidence") or 70.0)

        if budget_exceeded:
            meta["decision"] = "review_required"
            meta["decision_reason"] = "budget_exceeded"
        elif avg_conf is None:
            meta["decision"] = "review_required"
            meta["decision_reason"] = "avg_confidence_missing"
        else:
            try:
                v = float(avg_conf)
                if 0.0 <= v <= 1.0:
                    v = v * 100.0

                meta["avg_confidence_normalized"] = v

                if v >= min_conf:
                    meta["decision"] = "accept"
                    meta["decision_reason"] = "confidence_ok"
                else:
                    meta["decision"] = "review_required"
                    meta["decision_reason"] = "confidence_below_threshold"
            except Exception:
                meta["decision"] = "review_required"
                meta["decision_reason"] = "avg_confidence_parse_failed"

        return ExtractResult(text=text, meta=meta)

    def _normalize(self, text: str) -> str:
        return (text or "").replace("\r\n", "\n").replace("\r", "\n").strip()

    def _to_paddle_lang(self, lang: str) -> str:
        lang = (lang or "jpn").strip().lower()
        if "jpn" in lang and "eng" in lang:
            return "japan"
        if "jpn" in lang:
            return "japan"
        if "eng" in lang:
            return "en"
        return "japan"

    def _normalize_paddle_result(self, result: Any) -> list[dict[str, Any]]:
        out: list[dict[str, Any]] = []

        if not result:
            return out

        candidates = result
        if isinstance(result, list) and len(result) == 1 and isinstance(result[0], list):
            candidates = result[0]

        for item in candidates or []:
            try:
                box_raw = item[0]
                text_raw = item[1][0]
                score_raw = item[1][1]

                box = self._flatten_box(box_raw)
                text = str(text_raw).strip()
                score = float(score_raw) if score_raw is not None else None

                out.append(
                    {
                        "box": box,
                        "text": text,
                        "score": score,
                    }
                )
            except Exception:
                continue

        return out

    def _flatten_box(self, box_raw: Any) -> list[int]:
        try:
            xs = []
            ys = []
            for pt in box_raw:
                xs.append(int(pt[0]))
                ys.append(int(pt[1]))
            return [min(xs), min(ys), max(xs), max(ys)]
        except Exception:
            return []

    def _safe_int(self, arr: list[Any], idx: int) -> int:
        try:
            return int(arr[idx])
        except Exception:
            return 0


def decode_and_verify(content_b64: str, content_sha256: str) -> bytes:
    try:
        data = base64.b64decode(content_b64)
    except Exception:
        raise ValueError("content_b64 decode failed")

    sha = hashlib.sha256(data).hexdigest()
    if sha != content_sha256:
        raise ValueError("content_sha256 mismatch")

    return data


def parse_options(options: dict[str, Any]) -> OcrOptions:
    engine = str(options.get("engine") or "auto")
    mode = str(options.get("mode") or "auto")
    lang = str(options.get("lang") or "jpn")
    dpi = int(options.get("dpi") or 200)
    max_pages = int(options.get("max_pages") or 3)
    min_len = int(options.get("min_length_for_no_ocr") or 200)
    min_conf = float(options.get("min_confidence") or 70.0)

    budget_obj = options.get("budget") or {}
    if not isinstance(budget_obj, dict):
        budget_obj = {}

    budget = Budget(
        max_ms=int(budget_obj.get("max_ms") or 3000),
        max_cost_usd=float(budget_obj.get("max_cost_usd") or 0.0),
    )

    if engine not in {"auto", "tesseract", "paddleocr", "doctr", "textract", "document_ai"}:
        engine = "auto"
    if mode not in {"auto", "force_ocr"}:
        mode = "auto"

    return OcrOptions(
        engine=engine,
        mode=mode,
        lang=lang,
        dpi=dpi,
        max_pages=max_pages,
        min_length_for_no_ocr=min_len,
        min_confidence=min_conf,
        budget=budget,
    )