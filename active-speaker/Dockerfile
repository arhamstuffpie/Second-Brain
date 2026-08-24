FROM python:3.10.16-slim-bookworm AS model

ARG FAST_ASD_COMMIT=a6cab6e92baedf70b1976b856ee7caeb417e604e
ARG TALKNET_COMMIT=6d6821479af485e251c4991487e40573b42181b4

RUN apt-get update \
    && apt-get install --yes --no-install-recommends git ca-certificates \
    && git clone https://github.com/sieve-community/fast-asd.git /opt/fast-asd \
    && git -C /opt/fast-asd checkout "$FAST_ASD_COMMIT" \
    && git clone https://github.com/TaoRuijie/TalkNet-ASD.git /opt/talknet-upstream \
    && git -C /opt/talknet-upstream checkout "$TALKNET_COMMIT" \
    && cp -R /opt/talknet-upstream/model/faceDetector/s3fd /opt/fast-asd/talknet/model/faceDetector/ \
    && cp /opt/talknet-upstream/LICENSE.md /opt/fast-asd/TALKNET-LICENSE.md \
    && rm -rf /opt/fast-asd/.git /opt/talknet-upstream /var/lib/apt/lists/*

FROM python:3.10.16-slim-bookworm

ARG ACTIVE_SPEAKER_DEVICE=cpu

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    ACTIVE_SPEAKER_RUNTIME_PATH=/opt/fast-asd/talknet \
    ACTIVE_SPEAKER_WORK_DIR=/tmp/active-speaker

RUN apt-get update \
    && apt-get install --yes --no-install-recommends curl ffmpeg libglib2.0-0 libgl1 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system detector \
    && useradd --system --gid detector --home-dir /srv/detector detector

WORKDIR /srv/detector
COPY requirements.lock.txt ./
RUN if [ "$ACTIVE_SPEAKER_DEVICE" = "cuda" ]; then \
      pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cu121 \
        --extra-index-url https://pypi.org/simple \
        torch==2.2.2 torchvision==0.17.2 torchaudio==2.2.2; \
    else \
      pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu \
        --extra-index-url https://pypi.org/simple \
        torch==2.2.2 torchvision==0.17.2 torchaudio==2.2.2; \
    fi \
    && pip install --no-cache-dir --requirement requirements.lock.txt

COPY --from=model /opt/fast-asd /opt/fast-asd
RUN mkdir -p /models /tmp/active-speaker \
    && gdown 1J-PDWDAkYCdT8T2Nxn3Q_-iOHH_t-9YP -O /models/pretrain_TalkSet.model \
    && gdown 1KafnHz7ccT-3IyddBsL5yi2xGtxAKypt -O /models/sfd_face.pth \
    && echo 'd985ddd07dca08864a28337726f5b7d91b6426bebc7327f0e9b21cf9ff1cc937  /models/pretrain_TalkSet.model' | sha256sum -c - \
    && echo 'd54a87c2b7543b64729c9a25eafd188da15fd3f6e02f0ecec76ae1b30d86c491  /models/sfd_face.pth' | sha256sum -c -
RUN sed -i 's|/root/.cache/models/pretrain_TalkSet.model|/models/pretrain_TalkSet.model|' /opt/fast-asd/talknet/demoTalkNet.py \
    && sed -i "s|PATH_WEIGHT = 'model/faceDetector/s3fd/sfd_face.pth'|PATH_WEIGHT = '/models/sfd_face.pth'|" /opt/fast-asd/talknet/model/faceDetector/s3fd/__init__.py \
    && sed -i 's/astype(np.int)/astype(int)/g' /opt/fast-asd/talknet/model/faceDetector/s3fd/box_utils.py \
    && if [ "$ACTIVE_SPEAKER_DEVICE" = "cpu" ]; then \
      sed -i 's/\.cuda()/\.cpu()/g; s/torch.load(path)/torch.load(path, map_location=torch.device("cpu"))/g' /opt/fast-asd/talknet/talkNet.py /opt/fast-asd/talknet/demoTalkNet.py; \
      sed -i "s/device='cuda'/device='cpu'/g" /opt/fast-asd/talknet/demoTalkNet.py; \
    fi \
    && chown -R detector:detector /srv/detector /tmp/active-speaker /models

COPY app ./app

USER detector
EXPOSE 8093
HEALTHCHECK --interval=30s --timeout=5s --start-period=180s --retries=5 \
    CMD curl --fail --silent http://127.0.0.1:8093/healthz >/dev/null || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8093", "--workers", "1", "--no-access-log"]
