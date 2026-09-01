FROM python:3.11.13-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYANNOTE_METRICS_ENABLED=0 \
    HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1 \
    AUDIO_ANALYZER_DEVICE=cpu \
    AUDIO_ANALYZER_WORK_DIR=/tmp/audio-analyzer

RUN apt-get update \
    && apt-get install --yes --no-install-recommends curl ffmpeg libgomp1 libsndfile1 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 analyzer \
    && useradd --system --uid 10001 --gid analyzer --home-dir /srv/analyzer analyzer

WORKDIR /srv/analyzer
COPY requirements.lock.txt ./
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu \
        torch==2.8.0 torchaudio==2.8.0 \
    && pip install --no-cache-dir --requirement requirements.lock.txt
COPY app ./app
COPY models /models
RUN mkdir -p /tmp/audio-analyzer \
    && chown -R analyzer:analyzer /srv/analyzer /tmp/audio-analyzer /models

USER analyzer
EXPOSE 8095
HEALTHCHECK --interval=30s --timeout=5s --start-period=180s --retries=5 \
    CMD curl --fail --silent http://127.0.0.1:8095/readyz >/dev/null || exit 1

CMD ["sh", "-c", "cd /tmp && exec uvicorn --app-dir /srv/analyzer app.main:app --host 0.0.0.0 --port 8095 --workers 1 --no-access-log"]
