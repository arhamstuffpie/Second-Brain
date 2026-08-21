# Local ML services

The capture identity pipeline uses three independent services:

| Port | Service | Question answered |
| --- | --- | --- |
| 8091 | speaker-embedder | Whose voice is this? |
| 8092 | face-embedder | Whose face is this? |
| 8093 | active-speaker | Which visible face is speaking now? |

## Run all three locally

Download the YuNet and SFace files once, then build the stack:

```bash
cd face-embedder
./download-models.sh
cd ..
docker compose -f compose.ml.yaml up --build -d
```

Check each service:

```bash
curl http://127.0.0.1:8091/healthz
curl http://127.0.0.1:8092/healthz
curl http://127.0.0.1:8093/healthz
```

Configure the backend:

```dotenv
APP_SPEAKER_EMBEDDING_PROVIDER=local
APP_SPEAKER_EMBEDDING_BASE_URL=http://127.0.0.1:8091
APP_SPEAKER_EMBEDDING_MODEL=speechbrain/spkrec-ecapa-voxceleb

APP_FACE_RECOGNITION_PROVIDER=local
APP_FACE_RECOGNITION_BASE_URL=http://127.0.0.1:8092
APP_FACE_RECOGNITION_MODEL=opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79

APP_ACTIVE_SPEAKER_PROVIDER=local
APP_ACTIVE_SPEAKER_BASE_URL=http://127.0.0.1:8093
APP_ACTIVE_SPEAKER_MODEL=talknet/talkset-v1

APP_ACTIVE_SPEAKER_AUTO_LINK=false
APP_PERSON_AUTO_MERGE=false
```

Enable automatic linking and merging only after reviewing several recordings in
`identity_link_evidence`.

## Use API keys without running models on the app machine

The simplest compatible option is to deploy this same Compose stack on a Linux
GPU server, put each service behind HTTPS, and protect all three with bearer
keys. The backend then uses the external mode:

```dotenv
APP_SPEAKER_EMBEDDING_PROVIDER=external
APP_SPEAKER_EMBEDDING_BASE_URL=https://speaker.example.com
APP_SPEAKER_EMBEDDING_API_KEY=replace-me

APP_FACE_RECOGNITION_PROVIDER=external
APP_FACE_RECOGNITION_BASE_URL=https://face.example.com
APP_FACE_RECOGNITION_API_KEY=replace-me

APP_ACTIVE_SPEAKER_PROVIDER=external
APP_ACTIVE_SPEAKER_BASE_URL=https://active-speaker.example.com
APP_ACTIVE_SPEAKER_API_KEY=replace-me
APP_ACTIVE_SPEAKER_MODEL=talknet/talkset-v1
```

Set the matching container-side keys using `SPEAKER_EMBEDDER_API_KEY`,
`FACE_EMBEDDER_API_KEY`, and `ACTIVE_SPEAKER_API_KEY`. The three values may be
the same secret if that is easier to operate, though separate keys make rotation
and revocation safer.

Hosted alternatives include Sieve's TalkNet deployment and NVIDIA Active
Speaker Detection. Their APIs do not match this repository's contracts, so a
provider adapter is required before their keys can be placed directly in the
backend configuration. Managed face and voice vendors have the same constraint:
`external` means contract-compatible, not automatically vendor-compatible.
