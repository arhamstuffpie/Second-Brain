# Active-speaker service

This service wraps the TalkNet active-speaker model using the optimized
`sieve-community/fast-asd` runtime. It implements the backend's existing
`GET /healthz` and multipart `POST /v1/active-speakers` contract.

## Start locally

CPU mode works on Docker Desktop but is slow. A Linux NVIDIA host is recommended
for recordings longer than a few minutes.

```bash
cd active-speaker
docker compose up --build -d
curl http://127.0.0.1:8093/healthz
```

Then configure the backend:

```dotenv
APP_ACTIVE_SPEAKER_PROVIDER=local
APP_ACTIVE_SPEAKER_BASE_URL=http://127.0.0.1:8093
APP_ACTIVE_SPEAKER_API_KEY=
APP_ACTIVE_SPEAKER_MODEL=talknet/talkset-v1
APP_ACTIVE_SPEAKER_AUTO_LINK=false
APP_PERSON_AUTO_MERGE=false
```

Keep automatic linking and merging disabled until you have reviewed the rows in
`identity_link_evidence` across multiple test recordings.

## GPU build

On a Linux host with the NVIDIA Container Toolkit:

```bash
ACTIVE_SPEAKER_DEVICE=cuda docker compose build
docker compose up -d
```

Add a GPU reservation to your local Compose override if your Docker setup does
not expose all GPUs by default.

## Current safety boundary

The backend currently sends person-track IDs and time ranges, but no spatial
face boxes. The service therefore emits evidence only when exactly one app
person track overlaps a diarized segment and TalkNet finds exactly one active
face track. Ambiguous segments are skipped rather than guessed.

Frames are stored on temporary disk by default to avoid holding a full recording
in memory. Set `ACTIVE_SPEAKER_IN_MEMORY_FRAMES` only after measuring memory use.

## Hosted/API-key alternatives

The backend supports `external` providers, but an external endpoint must still
implement this repository's `/healthz` and `/v1/active-speakers` JSON contract.
Hosted TalkNet services such as Sieve and NVIDIA Active Speaker Detection use
their own request/response formats, so they require a small adapter before their
API key can be used here.

The same rule applies to face and speaker embedding providers: set each backend
provider to `external` only when its endpoint implements the corresponding local
service contract. API keys alone are not interchangeable between vendors.
