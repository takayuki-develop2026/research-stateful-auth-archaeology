from fastapi import FastAPI
from sqlalchemy import text

from atlaskernel.db.session import engine
from atlaskernel.api.routes.reviews import router as review_router
from atlaskernel.api.routes.analyze import router as analyze_router
from atlaskernel.api.routes.extract import router as extract_router

app = FastAPI(
    title="AtlasKernel API",
    version="0.3.1",
    description="AtlasKernel Entity Analysis / OCR Extraction API",
)

app.include_router(review_router)
app.include_router(analyze_router)
app.include_router(extract_router)


@app.get("/health")
def health():
    try:
        with engine.connect() as conn:
            conn.execute(text("SELECT 1"))
        return {
            "status": "ok",
            "db": "ok",
            "service": "atlaskernel",
        }
    except Exception as e:
        return {
            "status": "ng",
            "db": "ng",
            "service": "atlaskernel",
            "error": str(e),
        }