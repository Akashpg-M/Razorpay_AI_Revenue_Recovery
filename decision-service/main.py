import time
import uuid
from collections import Counter, defaultdict

from fastapi import FastAPI, Request

from logging_config import configure_logging
from prediction.service import router as prediction_router
from prediction.natural_service import router as natural_prediction_router


logger = configure_logging()
COUNTERS = Counter()
DURATIONS = defaultdict(list)

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

    correlation_id = request.headers.get("x-correlation-id") or str(uuid.uuid4())
    try:
        response = await call_next(request)
        error_class = None
    except Exception as exc:
        COUNTERS[("decision_service_errors_total", type(exc).__name__)] += 1
        logger.exception("request_failed", extra={"service":"decision-service","event":"http_request_failed","correlation_id":correlation_id,"route":request.url.path,"error_class":type(exc).__name__})
        raise

    duration_ms = (
        time.perf_counter() - start
    ) * 1000

    route = request.scope.get("route")
    route_path = getattr(route, "path", request.url.path)
    COUNTERS[("decision_service_http_requests_total", request.method, route_path, str(response.status_code))] += 1
    DURATIONS[route_path].append(duration_ms)
    response.headers["X-Correlation-ID"] = correlation_id
    logger.info("http_request", extra={"service":"decision-service","event":"http_request","correlation_id":correlation_id,"route":route_path,"method":request.method,"status":response.status_code,"duration_ms":round(duration_ms,2),"error_class":error_class})

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

@app.get("/metrics")
async def metrics():
    from fastapi.responses import PlainTextResponse
    lines = []
    for labels, value in sorted(COUNTERS.items(), key=lambda item: str(item[0])):
        name, *parts = labels
        lines.append(f'{name}{{labels="{":".join(parts)}"}} {value}')
    for route, values in sorted(DURATIONS.items()):
        lines.append(f'decision_service_http_duration_milliseconds_count{{route="{route}"}} {len(values)}')
        lines.append(f'decision_service_http_duration_milliseconds_sum{{route="{route}"}} {sum(values):.3f}')
    return PlainTextResponse("\n".join(lines)+"\n", media_type="text/plain; version=0.0.4")
