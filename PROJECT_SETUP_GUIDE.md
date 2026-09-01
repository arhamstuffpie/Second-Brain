# Project setup guide

This guide runs the complete development stack:

| Component | Address | Purpose |
| --- | --- | --- |
| Memograph | `http://localhost:8080` | Graph memory service |
| Backend | `http://localhost:8181` | Go API and background workers |
| Speaker embedder | `http://127.0.0.1:8091` | Voice embeddings |
| Face embedder | `http://127.0.0.1:8092` | Face detection and embeddings |
| Active speaker | `http://127.0.0.1:8093` | Visible speaking-face detection |
| Expo | printed by Expo | Mobile development server |

## 1. Prerequisites

Install:

- Go 1.25 or newer
- Node.js and npm
- Docker Desktop with at least 10 GB available to the ML stack
- PostgreSQL
- `ffmpeg` and `ffprobe`
- Goose 3.25 or newer for database migrations
- sqlc 1.30 or newer only when regenerating database code
- A running Memograph instance and its API key or JWT
- An OpenAI API key for transcription and visual analysis, unless mock providers are used

Confirm the main tools:

```bash
go version
node --version
npm --version
docker version
ffmpeg -version
ffprobe -version
goose --version
```

## 2. Configure PostgreSQL

Use an existing database or create a local database and user. The resulting URL
must be placed in `APP_DATABASE_URL`:

```dotenv
APP_DATABASE_URL=postgres://app_user:change-me@localhost:5432/ai_second_brain?sslmode=disable
```

The repository does not start PostgreSQL automatically. Verify connectivity
before running migrations:

```bash
psql "$APP_DATABASE_URL" -c 'select 1'
```

## 3. Create the backend environment

From the repository root:

```bash
cp backend/.env.example backend/.env
```

At minimum, replace these placeholders in `backend/.env`:

```dotenv
APP_HTTP_PORT=8181
APP_DATABASE_URL=postgres://app_user:change-me@localhost:5432/ai_second_brain?sslmode=disable
APP_JWT_SECRET=replace-with-at-least-32-random-characters
APP_MODEL_CREDENTIAL_KEY=replace-with-a-different-stable-32-character-secret

APP_STT_PROVIDER=openai
APP_STT_API_KEY=replace-me
APP_STT_MODEL=gpt-4o-transcribe-diarize

APP_VISION_PROVIDER=openai
APP_VISION_API_KEY=replace-me
APP_VISION_MODEL=gpt-4.1-mini

APP_MEMOGRAPH_BASE_URL=http://localhost:8080
APP_MEMOGRAPH_API_KEY=replace-me
APP_MEMOGRAPH_JWT=
```

Use either the Memograph API key or JWT. Never commit `backend/.env`, and do not
paste real credentials into source files, screenshots, issues, or chat.

For an offline backend smoke test, use `APP_STT_PROVIDER=mock`,
`APP_VISION_PROVIDER=mock`, and leave Memograph variables empty. Capture cannot
write graph memories in that mode.

## 4. Start the three identity services

Download the face models once, then start the combined stack:

```bash
./face-embedder/download-models.sh
docker compose -f compose.ml.yaml up --build -d
```

Enable the services in `backend/.env`:

```dotenv
APP_SPEAKER_EMBEDDING_PROVIDER=local
APP_SPEAKER_EMBEDDING_BASE_URL=http://127.0.0.1:8091
APP_SPEAKER_EMBEDDING_API_KEY=
APP_SPEAKER_EMBEDDING_MODEL=speechbrain/spkrec-ecapa-voxceleb

APP_FACE_RECOGNITION_PROVIDER=local
APP_FACE_RECOGNITION_BASE_URL=http://127.0.0.1:8092
APP_FACE_RECOGNITION_API_KEY=
APP_FACE_RECOGNITION_MODEL=opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
APP_FACE_AUTO_CONFIRM=false

APP_ACTIVE_SPEAKER_PROVIDER=local
APP_ACTIVE_SPEAKER_BASE_URL=http://127.0.0.1:8093
APP_ACTIVE_SPEAKER_API_KEY=
APP_ACTIVE_SPEAKER_MODEL=talknet/talkset-v1
APP_ACTIVE_SPEAKER_TIMEOUT=15m
APP_ACTIVE_SPEAKER_AUTO_LINK=false
APP_PERSON_AUTO_MERGE=false
```

The speaker and face services run comfortably on CPU. TalkNet also works on
CPU, but processing is slower than recording duration. Use a Linux NVIDIA GPU
for production or near-real-time throughput.

Check all three services:

```bash
curl http://127.0.0.1:8091/healthz
curl http://127.0.0.1:8092/healthz
curl http://127.0.0.1:8093/healthz
docker compose -f compose.ml.yaml ps
```

View or stop the stack:

```bash
docker compose -f compose.ml.yaml logs -f
docker compose -f compose.ml.yaml down
```

## 5. Apply database migrations

The Makefile migration command uses `DATABASE_URL`, not `APP_DATABASE_URL`:

```bash
make migrate-up DATABASE_URL='postgres://app_user:change-me@localhost:5432/ai_second_brain?sslmode=disable'
```

Confirm the identity tables exist:

```sql
SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name IN (
    'voice_speaker_profiles',
    'face_profiles',
    'face_profile_samples',
    'person_profiles',
    'person_tracks',
    'identity_link_evidence'
  )
ORDER BY table_name;
```

Do not manually insert face or person profiles. They are created from accepted
capture evidence and explicit enrollment flows.

## 6. Start the backend

The root Makefile loads `backend/.env`:

```bash
make run
```

In another terminal:

```bash
curl http://localhost:8181/health
```

The backend must be restarted after changing `backend/.env`.

## 7. Start the mobile application

Install dependencies once:

```bash
cd app
npm install
```

For an iOS simulator or web development:

```bash
EXPO_PUBLIC_API_BASE_URL=http://localhost:8181 npx expo start
```

For a physical phone, replace `YOUR_COMPUTER_LAN_IP` and keep the phone and
computer on the same network:

```bash
EXPO_PUBLIC_API_BASE_URL=http://YOUR_COMPUTER_LAN_IP:8181 npx expo start
```

`localhost` on a physical phone refers to the phone, not the development
computer. Camera capture should be tested on a physical device.

## 8. First identity test

1. Sign up or log in.
2. Enroll a clean 2–10 second owner voice sample.
3. Create or select a Memograph project and memory in Settings.
4. Start a capture session with one clearly visible, well-lit face.
5. Let the person speak for several separated utterances.
6. Stop capture and wait for background jobs to finish.
7. Inspect backend logs and the database evidence before testing multiple people.

Useful database checks:

```sql
SELECT * FROM voice_speaker_profiles ORDER BY created_at DESC;
SELECT * FROM face_profiles ORDER BY created_at DESC;
SELECT * FROM face_profile_samples ORDER BY created_at DESC;
SELECT * FROM person_profiles ORDER BY created_at DESC;
SELECT * FROM person_tracks ORDER BY created_at DESC;
SELECT * FROM identity_link_evidence ORDER BY created_at DESC;
```

Keep these disabled until several recordings produce correct evidence:

```dotenv
APP_FACE_AUTO_CONFIRM=false
APP_ACTIVE_SPEAKER_AUTO_LINK=false
APP_PERSON_AUTO_MERGE=false
```

## 9. Verification

Run backend checks:

```bash
make test
make vet
make build
```

Run mobile checks:

```bash
cd app
npm run typecheck
```

Validate Compose without starting containers:

```bash
docker compose -f compose.ml.yaml config --quiet
docker compose -f speaker-embedder/compose.yaml config --quiet
docker compose -f face-embedder/compose.yaml config --quiet
docker compose -f active-speaker/compose.yaml config --quiet
```

## 10. Common problems

### Backend cannot start

- Confirm PostgreSQL is reachable using the exact `APP_DATABASE_URL`.
- Ensure both credential secrets contain at least 32 characters.
- Install `ffmpeg` and verify `ffmpeg` and `ffprobe` are on `PATH`.
- A configured Memograph URL requires either an API key or JWT.

### The mobile app cannot reach the backend

- Use port `8181` for the backend, not Memograph's port `8080`.
- On a physical phone, use the computer's LAN IP instead of `localhost`.
- Check firewall permissions and confirm `curl http://LAN_IP:8181/health` works.

### Face tables remain empty

- Confirm the face service health endpoint succeeds.
- Use frontal, sharp, well-lit faces large enough in the frame.
- Inspect capture logs for rejected samples such as blur, pose, or low quality.
- `face_profiles` stays empty until a usable face sample reaches enrollment or
  identity resolution; merely starting the service does not create a profile.

### Active-speaker evidence is empty

- Confirm the video contains audio and a visible speaking face.
- CPU TalkNet may take several minutes; wait for the background job.
- Ambiguous scenes are intentionally skipped when more than one application
  person track overlaps the same speech segment.
- The current backend metadata does not provide per-frame face boxes, so safe
  linking is limited to unambiguous single-person overlap.

### Resetting after configuration changes

Restart only the affected component:

```bash
docker compose -f compose.ml.yaml restart active-speaker
```

Rebuild when the service source, dependencies, or Dockerfile changes:

```bash
docker compose -f compose.ml.yaml up --build -d active-speaker
```

Do not delete PostgreSQL data or biometric sample directories unless a full,
intentional reset is required and backups have been reviewed.
