import base64
import hashlib
import os
import tempfile
from typing import Any, Dict, List, Optional, Tuple

import cv2
import numpy as np
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

router = APIRouter(prefix="/v1/preprocess", tags=["preprocess"])


class PreprocessRequest(BaseModel):
    content_b64: str
    filename: Optional[str] = None
    content_sha256: Optional[str] = None
    operations: List[str] = Field(default_factory=list)
    options: Dict[str, Any] = Field(default_factory=dict)


class PreprocessPageResponse(BaseModel):
    page: int
    media_type: str
    ext: str
    processed_content_b64: str
    processed_sha256: str
    bytes: int
    metadata: Dict[str, Any]


class PreprocessResponse(BaseModel):
    media_type: str
    ext: str
    processed_content_b64: str
    processed_sha256: str
    bytes: int
    metadata: Dict[str, Any]
    pages: List[PreprocessPageResponse] = Field(default_factory=list)


def _sha256_hex(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _decode_b64(data: str) -> bytes:
    try:
        return base64.b64decode(data)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"invalid content_b64: {e}")


def _encode_b64(data: bytes) -> str:
    return base64.b64encode(data).decode("utf-8")


def _infer_ext(filename: Optional[str]) -> str:
    if not filename:
        return ".bin"
    _, ext = os.path.splitext(filename.lower())
    return ext or ".bin"


def _is_image_ext(ext: str) -> bool:
    return ext in {".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tif", ".tiff"}


def _opencv_basic(img: np.ndarray) -> np.ndarray:
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    gray = cv2.equalizeHist(gray)
    return cv2.cvtColor(gray, cv2.COLOR_GRAY2BGR)


def _deblur_basic(img: np.ndarray) -> np.ndarray:
    blur = cv2.GaussianBlur(img, (0, 0), 1.2)
    return cv2.addWeighted(img, 1.5, blur, -0.5, 0)


def _denoise_basic(img: np.ndarray) -> np.ndarray:
    return cv2.fastNlMeansDenoisingColored(img, None, 3, 3, 7, 21)


def _deskew_basic(img: np.ndarray) -> Tuple[np.ndarray, float, bool]:
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    _, bw = cv2.threshold(gray, 0, 255, cv2.THRESH_BINARY_INV | cv2.THRESH_OTSU)

    coords = np.column_stack(np.where(bw > 0))
    if coords.shape[0] < 20:
        return img, 0.0, False

    rect = cv2.minAreaRect(coords[:, ::-1].astype(np.float32))
    angle = rect[-1]

    if angle < -45:
        angle += 90
    if angle > 45:
        angle -= 90
    if -0.3 < angle < 0.3:
        return img, 0.0, False

    h, w = img.shape[:2]
    center = (w // 2, h // 2)
    m = cv2.getRotationMatrix2D(center, angle, 1.0)
    rotated = cv2.warpAffine(
        img,
        m,
        (w, h),
        flags=cv2.INTER_CUBIC,
        borderMode=cv2.BORDER_REPLICATE,
    )
    return rotated, float(angle), True


def _apply_operations(img: np.ndarray, ops: List[str]) -> Tuple[np.ndarray, List[str]]:
    applied: List[str] = []
    out = img

    for op in ops:
        if op == "opencv_basic":
            out = _opencv_basic(out)
            applied.append(op)
        elif op == "deblur_basic":
            out = _deblur_basic(out)
            applied.append(op)
        elif op == "deskew_basic":
            out, angle, ok = _deskew_basic(out)
            if ok:
                applied.append(f"{op}({angle:.2f}deg)")
            else:
                applied.append(f"{op}(no_rotation)")
        elif op == "denoise_basic":
            out = _denoise_basic(out)
            applied.append(op)
        else:
            applied.append(f"{op}(unsupported)")

    return out, applied


def _encode_png_bytes(img: np.ndarray) -> bytes:
    ok, encoded = cv2.imencode(".png", img)
    if not ok:
        raise HTTPException(status_code=500, detail="failed to encode processed image")
    return encoded.tobytes()


def _load_pdf_pages_as_images(raw_pdf: bytes, dpi: int) -> List[np.ndarray]:
    try:
        from pdf2image import convert_from_bytes  # type: ignore

        pil_images = convert_from_bytes(raw_pdf, dpi=dpi)
        out: List[np.ndarray] = []
        for pil_img in pil_images:
            arr = np.array(pil_img)
            if arr.ndim == 2:
                arr = cv2.cvtColor(arr, cv2.COLOR_GRAY2BGR)
            else:
                arr = cv2.cvtColor(arr, cv2.COLOR_RGB2BGR)
            out.append(arr)
        if out:
            return out
    except Exception:
        pass

    try:
        import pypdfium2 as pdfium  # type: ignore

        out: List[np.ndarray] = []
        with tempfile.NamedTemporaryFile(suffix=".pdf", delete=True) as tf:
            tf.write(raw_pdf)
            tf.flush()

            pdf = pdfium.PdfDocument(tf.name)
            scale = dpi / 72.0
            for i in range(len(pdf)):
                page = pdf[i]
                bitmap = page.render(scale=scale)
                pil_img = bitmap.to_pil()
                arr = np.array(pil_img)
                if arr.ndim == 2:
                    arr = cv2.cvtColor(arr, cv2.COLOR_GRAY2BGR)
                else:
                    arr = cv2.cvtColor(arr, cv2.COLOR_RGB2BGR)
                out.append(arr)
        if out:
            return out
    except Exception:
        pass

    raise HTTPException(
        status_code=500,
        detail="pdf rasterize backend is not available. install pdf2image or pypdfium2",
    )


def _build_summary_placeholder(page_count: int, ops: List[str], dpi: int, original_page_count: int) -> Dict[str, Any]:
    return {
        "adapter": "python_preprocess_pdf_pages",
        "source_kind": "pdf",
        "dpi": dpi,
        "page_count": page_count,
        "original_page_count": original_page_count,
        "truncated": original_page_count > page_count,
        "operations": ops,
        "selection": ops,
        "delivery_mode": "pages",
    }


@router.post("")
def preprocess(req: PreprocessRequest):
    raw = _decode_b64(req.content_b64)
    ext = _infer_ext(req.filename)
    ops = req.operations or ["opencv_basic"]
    options = req.options or {}

    if req.content_sha256:
        actual = _sha256_hex(raw)
        if actual != req.content_sha256:
            raise HTTPException(
                status_code=400,
                detail=f"content_sha256 mismatch: expected={req.content_sha256} actual={actual}",
            )

    # -----------------------------
    # PDF path: return pages[]
    # -----------------------------
    if ext == ".pdf":
        dpi = int(options.get("dpi", 200) or 200)
        max_pages = int(options.get("max_pages", 20) or 20)
        page_images = _load_pdf_pages_as_images(raw, dpi=dpi)

        if not page_images:
            raise HTTPException(status_code=500, detail="pdf rasterize returned no pages")

        original_page_count = len(page_images)
        page_images = page_images[: max_pages]

        pages: List[PreprocessPageResponse] = []
        page_operation_logs: List[Dict[str, Any]] = []

        for idx, page_img in enumerate(page_images, start=1):
            processed, applied = _apply_operations(page_img, ops)
            out = _encode_png_bytes(processed)
            out_sha = _sha256_hex(out)

            page_meta = {
                "page": idx,
                "operations": applied,
                "selection": ops,
                "width": int(processed.shape[1]),
                "height": int(processed.shape[0]),
                "adapter": "python_preprocess_pdf_page",
                "source_kind": "pdf_page",
                "dpi": dpi,
            }

            pages.append(
                PreprocessPageResponse(
                    page=idx,
                    media_type="image/png",
                    ext="png",
                    processed_content_b64=_encode_b64(out),
                    processed_sha256=out_sha,
                    bytes=len(out),
                    metadata=page_meta,
                )
            )

            page_operation_logs.append(
                {
                    "page": idx,
                    "operations": applied,
                    "width": int(processed.shape[1]),
                    "height": int(processed.shape[0]),
                }
            )

        summary_meta = _build_summary_placeholder(
            page_count=len(page_images),
            ops=ops,
            dpi=dpi,
            original_page_count=original_page_count,
        )
        summary_meta["page_operations"] = page_operation_logs

        return PreprocessResponse(
            media_type="application/x-atlas-pages+json",
            ext="json",
            processed_content_b64="",
            processed_sha256="",
            bytes=0,
            metadata=summary_meta,
            pages=pages,
        )

    # -----------------------------
    # Image path: still single page, but also populate pages[]
    # -----------------------------
    if not _is_image_ext(ext):
        raise HTTPException(status_code=400, detail=f"unsupported extension: {ext}")

    arr = np.frombuffer(raw, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise HTTPException(status_code=400, detail="failed to decode image")

    processed, applied = _apply_operations(img, ops)
    out = _encode_png_bytes(processed)
    out_sha = _sha256_hex(out)

    single_meta = {
        "adapter": "python_preprocess",
        "source_kind": "image",
        "operations": applied,
        "selection": ops,
        "width": int(processed.shape[1]),
        "height": int(processed.shape[0]),
    }

    return PreprocessResponse(
        media_type="image/png",
        ext="png",
        processed_content_b64=_encode_b64(out),
        processed_sha256=out_sha,
        bytes=len(out),
        metadata=single_meta,
        pages=[
            PreprocessPageResponse(
                page=1,
                media_type="image/png",
                ext="png",
                processed_content_b64=_encode_b64(out),
                processed_sha256=out_sha,
                bytes=len(out),
                metadata={**single_meta, "page": 1},
            )
        ],
    )