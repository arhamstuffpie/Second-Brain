# ECAPA speaker embedder

This private service converts a clean 2–10 second mono speech clip into a
normalized ECAPA-TDNN vector. It does not transcribe audio and it does not know
speaker names. The Go API owns matching, profile lifecycle, and authorization.

## Run locally

From this directory:

```bash
docker compose up --build -d
curl http://127.0.0.1:8091/healthz
```

The first build/start downloads the SpeechBrain ECAPA model and can take a few
minutes. CPU inference is sufficient for the application's background jobs;
allow roughly 3 GB RAM. Set the backend environment shown below, apply database
migrations, and restart the Go API:

```dotenv
APP_SPEAKER_EMBEDDING_PROVIDER=local
APP_SPEAKER_EMBEDDING_BASE_URL=http://127.0.0.1:8091
APP_SPEAKER_EMBEDDING_MODEL=speechbrain/spkrec-ecapa-voxceleb
APP_SPEAKER_EMBEDDING_API_KEY=
```

To authenticate even on localhost, set the same random value in
`SPEAKER_EMBEDDER_API_KEY` and `APP_SPEAKER_EMBEDDING_API_KEY`.

## Run as an external provider

Deploy this image behind HTTPS on your container platform, set a strong
`SPEAKER_EMBEDDER_API_KEY`, and keep one replica warm so every request uses the
same model version. Configure the Go API with:

```dotenv
APP_SPEAKER_EMBEDDING_PROVIDER=external
APP_SPEAKER_EMBEDDING_BASE_URL=https://speaker-embedder.example.com
APP_SPEAKER_EMBEDDING_API_KEY=replace-with-a-managed-secret
APP_SPEAKER_EMBEDDING_MODEL=speechbrain/spkrec-ecapa-voxceleb
```

Any external service can replace this container if it implements
`POST /v1/embeddings` as multipart form data with `file` and `model`, accepts
`Authorization: Bearer <key>`, and returns:

```json
{
  "embedding": [0.012, -0.034],
  "model": "speechbrain/spkrec-ecapa-voxceleb",
  "dimensions": 192
}
```

Do not change the model identifier after profiles exist. Vectors from different
model families are intentionally isolated and cannot be compared safely.

Speaker samples and vectors are biometric data. In production, keep the API and
database on private networks, use encrypted database disks and an encrypted
sample volume/object store, restrict backups, rotate API keys, and document
consent/deletion in your privacy policy. The application enforces account scope,
hard-deletes manually removed profiles, and purges expired provisional profiles;
infrastructure encryption and backup retention remain deployment responsibilities.
