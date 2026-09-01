import time

from fastapi import FastAPI, Request

from logging_config import configure_logging
from prediction.service import router as prediction_router
from prediction.natural_service import router as natural_prediction_router


logger = configure_logging()

app = FastAPI(
    title="Revenue Recovery Decision Service",
    version="0.1.0",
)

app.include_router(prediction_router)
app.include_router(natural_prediction_router)


@app.middleware("http")
async def request_logging(
    request: Request,
    call_next,
):
    start = time.perf_counter()

    response = await call_next(request)

    duration_ms = (
        time.perf_counter() - start
    ) * 1000

    logger.info(
        f"http_request "
        f"method={request.method} "
        f"path={request.url.path} "
        f"status={response.status_code} "
        f"duration_ms={duration_ms:.2f}"
    )

    return response


@app.get("/health/live")
async def liveness():
    return {
        "service": "decision-service",
        "status": "ok",
    }


@app.get("/health/ready")
async def readiness():
    return {
        "service": "decision-service",
        "status": "ready",
    }
