from __future__ import annotations

import sys
from typing import Any, Dict, Optional

from fastapi import APIRouter, Depends
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from atlaskernel.db.session import get_db
from atlaskernel.application.analyze_entity import analyze as analyze_entity
from atlaskernel.domain.request import AnalysisRequest

router = APIRouter(prefix="/v1", tags=["analyze"])


class AnalyzeRequest(BaseModel):
    project_id: str
    task_type: str = "entity_extract"
    raw_text: str
    mode: int = 2
    assets_ref: Optional[str] = None

    # ✅ v3不変条件：mutable default を根絶（リクエスト間汚染を0に）
    context: Dict[str, Any] = Field(default_factory=dict)


@router.post("/analyze/entity")
def analyze_entity_endpoint(
    body: AnalyzeRequest,
    db: Session = Depends(get_db),
) -> Dict[str, Any]:
    print("[AK][ROUTE] hit /v1/analyze/entity", file=sys.stderr, flush=True)

    req = AnalysisRequest(
        # ✅ v3固定：entity_type と task_type を混ぜない（将来 task_type 拡張でも破綻しない）
        entity_type=body.task_type,
        raw_value=body.raw_text,
        project_id=body.project_id,
        task_type=body.task_type,
        mode=body.mode,
        known_assets_ref=body.assets_ref,
        context=body.context,  # default_factory で必ず dict
    )

    return analyze_entity(req)