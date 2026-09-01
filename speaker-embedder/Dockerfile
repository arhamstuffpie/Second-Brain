FROM python:3.11.13-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    SPEAKER_EMBEDDER_CACHE_DIR=/var/cache/speaker-embedder

RUN apt-get update \
    && apt-get install --yes --no-install-recommends libgomp1 libsndfile1 curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system embedder \
    && useradd --system --gid embedder --home-dir /srv/embedder embedder

WORKDIR /srv/embedder
COPY requirements.lock.txt ./
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu \
        torch==2.7.1 torchaudio==2.7.1 \
    && pip install --no-cache-dir --requirement requirements.lock.txt
COPY app ./app

RUN mkdir -p /var/cache/speaker-embedder \
    && chown -R embedder:embedder /srv/embedder /var/cache/speaker-embedder

USER embedder
EXPOSE 8091
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:8091/healthz >/dev/null || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8091", "--workers", "1", "--no-access-log"]
