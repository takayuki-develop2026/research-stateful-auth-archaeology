from __future__ import annotations

from typing import Any, Optional

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from atlaskernel.application.extract.ocr_router import (
    OcrRouter,
    decode_and_verify,
    parse_options,
    OcrOptions,
    ExtractResult,
)

router = APIRouter(tags=["extract"])


# =========================
# Request / Response Models
# =========================

class ExtractBaseRequest(BaseModel):
    content_b64: str
    content_sha256: str
    source_url: Optional[str] = None
    options: dict[str, Any] = {}


class ExtractResponse(BaseModel):
    text: str
    meta: dict[str, Any]


# =========================
# Internal helpers
# =========================

def _to_http_error(e: Exception) -> HTTPException:
    msg = str(e) or e.__class__.__name__
    if "content_b64 decode failed" in msg or "content_sha256 mismatch" in msg:
        return HTTPException(status_code=400, detail=msg)
    return HTTPException(status_code=500, detail=msg)


def _run_pdf_router(
    *,
    pdf_bytes: bytes,
    source_url: Optional[str],
    opt: OcrOptions,
    mode_override: Optional[str] = None,
) -> ExtractResult:
    router_impl = OcrRouter()

    if mode_override is not None:
        opt = OcrOptions(
            engine=opt.engine,
            mode=mode_override,  # type: ignore[arg-type]
            lang=opt.lang,
            dpi=opt.dpi,
            max_pages=opt.max_pages,
            min_length_for_no_ocr=opt.min_length_for_no_ocr,
            min_confidence=opt.min_confidence,
            budget=opt.budget,
        )

    return router_impl.route(pdf_bytes, source_url, opt)


# =========================
# Endpoints
# =========================

@router.post("/v1/extract/pdf_text", response_model=ExtractResponse)
def extract_pdf_text(req: ExtractBaseRequest) -> ExtractResponse:
    try:
        pdf_bytes = decode_and_verify(req.content_b64, req.content_sha256)
        opt = parse_options(req.options or {})
        r = OcrRouter().extract_pdf_text(pdf_bytes, req.source_url, opt)

        meta = dict(r.meta)
        meta.setdefault("engine_requested", opt.engine)
        meta.setdefault("mode", opt.mode)
        meta.setdefault("lang", opt.lang)
        meta.setdefault("min_confidence", opt.min_confidence)
        meta.setdefault("budget", {"max_ms": opt.budget.max_ms, "max_cost_usd": opt.budget.max_cost_usd})
        meta.setdefault("pipeline", "pdf_text_only")
        meta.setdefault("fallback_chain", ["pdf_text"])

        return ExtractResponse(text=r.text, meta=meta)
    except Exception as e:
        raise _to_http_error(e)


@router.post("/v1/extract/pdf_ocr", response_model=ExtractResponse)
def extract_pdf_ocr(req: ExtractBaseRequest) -> ExtractResponse:
    try:
        pdf_bytes = decode_and_verify(req.content_b64, req.content_sha256)
        opt = parse_options(req.options or {})
        r = _run_pdf_router(pdf_bytes=pdf_bytes, source_url=req.source_url, opt=opt, mode_override="force_ocr")

        meta = dict(r.meta)
        meta.setdefault("pipeline", "pdf_ocr_only")
        meta.setdefault("fallback_chain", ["pdf_ocr"])
        meta.setdefault("engine_requested", opt.engine)
        meta.setdefault("mode", "force_ocr")
        meta.setdefault("lang", opt.lang)
        meta.setdefault("min_confidence", opt.min_confidence)
        meta.setdefault("budget", {"max_ms": opt.budget.max_ms, "max_cost_usd": opt.budget.max_cost_usd})

        return ExtractResponse(text=r.text, meta=meta)
    except Exception as e:
        raise _to_http_error(e)


@router.post("/v1/extract/pdf_extract", response_model=ExtractResponse)
def extract_pdf_extract(req: ExtractBaseRequest) -> ExtractResponse:
    try:
        pdf_bytes = decode_and_verify(req.content_b64, req.content_sha256)
        opt = parse_options(req.options or {})
        r = _run_pdf_router(pdf_bytes=pdf_bytes, source_url=req.source_url, opt=opt, mode_override=None)
        return ExtractResponse(text=r.text, meta=dict(r.meta))
    except Exception as e:
        raise _to_http_error(e)


@router.post("/v1/extract/image_ocr", response_model=ExtractResponse)
def extract_image_ocr(req: ExtractBaseRequest) -> ExtractResponse:
    """
    preprocess 後の PNG/JPEG/WebP/BMP/TIFF などの画像 bytes を直接 OCR する endpoint
    """
    try:
        image_bytes = decode_and_verify(req.content_b64, req.content_sha256)
        opt = parse_options(req.options or {})
        r = OcrRouter().extract_image_ocr(image_bytes, req.source_url, opt)

        meta = dict(r.meta)
        meta.setdefault("pipeline", "image_ocr_only")
        meta.setdefault("fallback_chain", ["image_ocr"])
        meta.setdefault("engine_requested", opt.engine)
        meta.setdefault("mode", "force_ocr")
        meta.setdefault("lang", opt.lang)
        meta.setdefault("min_confidence", opt.min_confidence)
        meta.setdefault("budget", {"max_ms": opt.budget.max_ms, "max_cost_usd": opt.budget.max_cost_usd})

        return ExtractResponse(text=r.text, meta=meta)
    except Exception as e:
        raise _to_http_error(e)