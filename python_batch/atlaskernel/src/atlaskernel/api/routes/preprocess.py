import base64
import hashlib
import os
from typing import Any, Dict, List, Optional

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


class PreprocessResponse(BaseModel):
    media_type: str
    ext: str
    processed_content_b64: str
    processed_sha256: str
    bytes: int
    metadata: Dict[str, Any]


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


def _deskew_basic(img: np.ndarray) -> tuple[np.ndarray, float, bool]:
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

    (h, w) = img.shape[:2]
    center = (w // 2, h // 2)
    m = cv2.getRotationMatrix2D(center, angle, 1.0)
    rotated = cv2.warpAffine(img, m, (w, h), flags=cv2.INTER_CUBIC, borderMode=cv2.BORDER_REPLICATE)
    return rotated, float(angle), True


@router.post("")
def preprocess(req: PreprocessRequest):
    raw = _decode_b64(req.content_b64)
    ext = _infer_ext(req.filename)
    ops = req.operations or ["opencv_basic"]

    if req.content_sha256:
        actual = _sha256_hex(raw)
        if actual != req.content_sha256:
            raise HTTPException(
                status_code=400,
                detail=f"content_sha256 mismatch: expected={req.content_sha256} actual={actual}",
            )

    # PDF はいったん passthrough
    if ext == ".pdf":
        out_sha = _sha256_hex(raw)
        return PreprocessResponse(
            media_type="application/pdf",
            ext="pdf",
            processed_content_b64=_encode_b64(raw),
            processed_sha256=out_sha,
            bytes=len(raw),
            metadata={
                "adapter": "python_preprocess_pdf_passthrough",
                "operations": ops,
                "selection": ops,
            },
        )

    if not _is_image_ext(ext):
        raise HTTPException(status_code=400, detail=f"unsupported extension: {ext}")

    arr = np.frombuffer(raw, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise HTTPException(status_code=400, detail="failed to decode image")

    applied: List[str] = []
    for op in ops:
        if op == "opencv_basic":
            img = _opencv_basic(img)
            applied.append(op)
        elif op == "deblur_basic":
            img = _deblur_basic(img)
            applied.append(op)
        elif op == "deskew_basic":
            img, angle, ok = _deskew_basic(img)
            if ok:
                applied.append(f"{op}({angle:.2f}deg)")
            else:
                applied.append(f"{op}(no_rotation)")
        elif op == "denoise_basic":
            img = _denoise_basic(img)
            applied.append(op)
        else:
            applied.append(f"{op}(unsupported)")

    ok, encoded = cv2.imencode(".png", img)
    if not ok:
        raise HTTPException(status_code=500, detail="failed to encode processed image")

    out = encoded.tobytes()
    out_sha = _sha256_hex(out)

    return PreprocessResponse(
        media_type="image/png",
        ext="png",
        processed_content_b64=_encode_b64(out),
        processed_sha256=out_sha,
        bytes=len(out),
        metadata={
            "adapter": "python_preprocess",
            "operations": applied,
            "selection": ops,
        },
    )