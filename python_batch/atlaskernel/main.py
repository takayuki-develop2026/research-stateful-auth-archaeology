from __future__ import annotations

from fastapi import FastAPI

from atlaskernel.application.analyze_entity import analyze
from atlaskernel.adapters.mysql_reader import read_requests_from_db
from atlaskernel.adapters.mysql_writer import write_results_to_db
from atlaskernel.api.routes.reviews import router as review_router
from atlaskernel.api.routes.extract import router as extract_router

app = FastAPI(
    title="AtlasKernel API",
    version="0.1.0",
    description="AtlasKernel Entity Analysis / OCR Extraction API",
)

app.include_router(review_router)
app.include_router(extract_router)


@app.get("/health")
def health() -> dict[str, object]:
    return {
        "ok": True,
        "service": "atlaskernel",
    }


def main() -> None:
    requests = read_requests_from_db()
    pairs = []

    for item_id, request in requests:
        results = analyze(request)

        # analyze() の戻りが dict でも list でも落ちないようにしておく
        if isinstance(results, list):
            for result in results:
                pairs.append((item_id, result))
        else:
            pairs.append((item_id, results))

    write_results_to_db(pairs)
    print("[OK] AtlasKernel DB pipeline executed")


if __name__ == "__main__":
    main()