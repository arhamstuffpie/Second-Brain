FROM python:3.11.13-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PERSON_ANALYZER_DEVICE=cpu \
    PERSON_ANALYZER_WORK_DIR=/tmp/person-analyzer

RUN apt-get update \
    && apt-get install --yes --no-install-recommends curl libglib2.0-0 libgl1 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 analyzer \
    && useradd --system --uid 10001 --gid analyzer --home-dir /srv/analyzer analyzer

WORKDIR /srv/analyzer
COPY requirements.lock.txt ./
RUN pip install --no-cache-dir --requirement requirements.lock.txt
COPY app ./app
RUN mkdir -p /tmp/person-analyzer \
    && chown -R analyzer:analyzer /srv/analyzer /tmp/person-analyzer

USER analyzer
EXPOSE 8094
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl --fail --silent http://127.0.0.1:8094/readyz >/dev/null || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8094", "--workers", "1", "--no-access-log"]
