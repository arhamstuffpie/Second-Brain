# AI Second Brain API

Production-oriented Go REST API skeleton using Gin, PostgreSQL, sqlc, Goose,
Zerolog, constructor dependency injection, and graceful shutdown. The complete
example endpoint is `GET /health`, which runs a sqlc-generated PostgreSQL probe.

## Architecture

`cmd/server/main.go` is the only composition root. It creates each dependency
exactly once and passes it downward:

```text
PostgreSQL -> repositories -> services -> handlers -> router
```

- HTTP handlers only translate HTTP input/output and invoke one use case.
- Services own business logic, authorization, transactions, and orchestration.
- Repositories own database operations; core queries use sqlc and the durable
  worker uses focused PostgreSQL queue statements.
- Services depend on small interfaces declared in the service package.
- Request contexts flow through every layer and JWT claims are attached to the
  standard `context.Context` as an authenticated principal.

The directory name `db/quries` intentionally follows the spelling requested in
the project structure. sqlc outputs generated Go code to `internal/db/sqlc`.

## Voice-to-Memograph pipeline

The service accepts recorded audio files and live-stream audio chunks,
transcribes them asynchronously, turns timestamped speech into natural-language
episodes, and writes each episode into a Memograph graph memory.

```text
upload/chunk -> local audio store -> PostgreSQL STT job
             -> timestamped transcript -> 30s episode buckets
             -> independent PostgreSQL Memograph jobs -> graph memory
```

The speech-to-text boundary is an interface, so another provider can replace the
included OpenAI-compatible adapter. PostgreSQL is the durable queue: workers
claim rows with `FOR UPDATE SKIP LOCKED`, and STT and Memograph jobs have
separate retry lifecycles. A failed graph write therefore does not transcribe
the audio again.

The production adapter calls `POST /v1/audio/transcriptions`. With
`gpt-4o-transcribe-diarize`, it requests `diarized_json` and automatic chunking;
`whisper-1` requests `verbose_json` segment timestamps, and other compatible
models use JSON output. Use `APP_STT_PROVIDER=mock` without external STT
credentials (the mock treats uploaded UTF-8 file contents as the transcript).

### Voice API

All routes require this application's JWT in
`Authorization: Bearer <access_token>`.

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/voice/recordings` | Upload a complete audio file |
| `POST` | `/api/v1/voice/chunks` | Upload one live-stream chunk |
| `GET` | `/api/v1/voice/recordings/:recording_id` | Poll transcript, episodes, and write status |
| `POST` | `/api/v1/voice/realtime/sessions` | Start a persistent microphone session |
| `POST` | `/api/v1/voice/realtime/sessions/:session_id/chunks` | Upload an ordered realtime chunk |
| `GET` | `/api/v1/voice/realtime/sessions/:session_id` | Read aggregate realtime progress |
| `POST` | `/api/v1/voice/realtime/sessions/:session_id/stop` | Stop a microphone session |
| `POST` | `/api/v1/voice/projects/:project_id/memories` | Create a graph memory with `create-full` |
| `POST` | `/api/v1/voice/memories/:memory_id/search` | Search, optionally scoped by `group_id` |
| `POST` | `/api/v1/voice/memories/:memory_id/answer` | Answer, with `group_id` enforced as a graph filter |
| `GET` | `/api/v1/voice/memories/:memory_id/graph` | Read the graph, optionally scoped by `group_id` |

Upload a recording:

```bash
curl -X POST http://localhost:8080/api/v1/voice/recordings \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@meeting.webm" \
  -F "session_id=session-123" \
  -F "group_id=user-456" \
  -F "memory_id=$MEMORY_ID" \
  -F "device_id=browser-mic" \
  -F "location=home-office"
```

`group_id` defaults to `session_id`. For a stream chunk, use `/chunks` with the
same fields and add `start_time` as the chunk offset in seconds. Both routes
return `202 Accepted` with a recording ID. Poll it until `status` is `completed`
or `failed`.

### Realtime 30-second microphone sessions

For browser microphone capture, use ordinary HTTPS uploads rather than a
WebSocket. Each chunk is an independent, retryable multipart request and enters
the same STT and Memograph queues as a complete recording.

Start a session:

```bash
curl -X POST http://localhost:8080/api/v1/voice/realtime/sessions \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "memory_id": "'"$MEMORY_ID"'",
    "group_id": "user-456",
    "device_id": "browser-mic",
    "location": "home-office",
    "chunk_duration_seconds": 30
  }'
```

Upload an ordered chunk:

```bash
curl -X POST \
  http://localhost:8080/api/v1/voice/realtime/sessions/$SESSION_ID/chunks \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@chunk-0.webm" \
  -F "chunk_index=0" \
  -F "is_final=false"
```

The server calculates the chunk start time as
`chunk_index * chunk_duration_seconds`. Repeating a chunk with the same session
and index returns the original recording instead of scheduling duplicate STT
or Memograph work. Send `is_final=true` on the last chunk, or call the explicit
stop endpoint.

A minimal browser loop can use a fresh `MediaRecorder` for each self-contained
30-second WebM/Opus file:

```js
let listening = false;
let recorder;
let stream;
let sessionId;
let chunkIndex = 0;

async function startListening(accessToken, memoryId) {
  const response = await fetch("/api/v1/voice/realtime/sessions", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      memory_id: memoryId,
      chunk_duration_seconds: 30,
      device_id: "browser-mic",
    }),
  });
  const payload = await response.json();
  sessionId = payload.data.id;
  stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  listening = true;
  recordNextChunk(accessToken);
}

function recordNextChunk(accessToken) {
  const parts = [];
  recorder = new MediaRecorder(stream, { mimeType: "audio/webm;codecs=opus" });
  recorder.ondataavailable = event => {
    if (event.data.size) parts.push(event.data);
  };
  recorder.onstop = async () => {
    const currentIndex = chunkIndex++;
    const isFinal = !listening;
    const blob = new Blob(parts, { type: "audio/webm" });
    if (listening) recordNextChunk(accessToken);
    else stream?.getTracks().forEach(track => track.stop());

    const form = new FormData();
    form.append("file", blob, `chunk-${currentIndex}.webm`);
    form.append("chunk_index", String(currentIndex));
    form.append("is_final", String(isFinal));
    await fetch(`/api/v1/voice/realtime/sessions/${sessionId}/chunks`, {
      method: "POST",
      headers: { Authorization: `Bearer ${accessToken}` },
      body: form,
    });
  };
  recorder.start();
  setTimeout(() => {
    if (recorder.state === "recording") recorder.stop();
  }, 30_000);
}

function stopListening() {
  listening = false;
  if (recorder?.state === "recording") recorder.stop();
}
```

Production UI should retry failed uploads with the same `chunk_index`, display
a persistent microphone indicator, require explicit user consent, and use
HTTPS because browsers restrict microphone access on insecure origins.

Each Memograph episode contains a description similar to:

```text
Audio memory from session session-123 between 0.00s and 18.40s.
Recorded at home-office. A said: "Let's ship on Friday."
```

Metadata includes `source`, `session_id`, `group_id`, `start_time`, `end_time`,
`location`, and `device_id`. Confidence is sent as a top-level custom field
when available.

### Create and query graph memory

The graph configuration is set once through Memograph's `create-full` route.
Template, instruction, and custom modes are accepted. The endpoint always sets
`memory_type` to `graph`, defaults the embedding model, and adds the
`confidence` float custom field if absent.

```bash
curl -X POST \
  http://localhost:8080/api/v1/voice/projects/$PROJECT_ID/memories \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Personal Voice Memory",
    "graph_config": {
      "mode": "custom",
      "entity_types": {
        "Person": "A person mentioned in speech",
        "Location": "A place mentioned in speech",
        "Activity": "An action or event described in speech"
      },
      "entity_type_colors": {
        "Person": "#3B82F6",
        "Location": "#10B981",
        "Activity": "#F59E0B"
      },
      "edge_types": {
        "PARTICIPATED_IN": "A person participated in an activity",
        "OCCURRED_AT": "An activity occurred at a location"
      },
      "edge_type_map": {
        "Person": ["PARTICIPATED_IN"],
        "Activity": ["OCCURRED_AT"]
      }
    }
  }'
```

Scoped search:

```bash
curl -X POST \
  http://localhost:8080/api/v1/voice/memories/$MEMORY_ID/search \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query":"What did we decide?", "limit":10, "group_id":"session-123"}'
```

The wrapper uses `X-Api-Key` for insert/search/answer when
`APP_MEMOGRAPH_API_KEY` is configured, falling back to the bearer JWT.
`create-full` and complete graph reads require `APP_MEMOGRAPH_JWT`, matching
the current Memograph route contracts. Configure both credentials when this
service must perform every operation. The unprefixed `MEMOGRAPH_API_KEY`,
`MEMOGRAPH_JWT_TOKEN`, and `MEMOGRAPH_BASE_URL` names are accepted as aliases.

## Video-to-Memograph pipeline

Video uploads and realtime chunks run through four independently retryable job
types:

```text
video upload -> audio extraction -> speech-to-text ----\
             -> frame extraction -> visual analysis ----> timeline merge
                                                       -> Memograph episode
```

FFmpeg extracts a mono WAV track and timestamped JPEG frames. The audio branch
uses the same swappable transcriber as voice ingestion. The visual branch uses
a `VisualAnalyzer` interface; the included OpenAI adapter sends image frames to
the Responses API with a strict JSON schema for objects, readable text,
activity, location, summary, and confidence. Raw video is never sent to the
vision model. Videos without an audio track are valid and produce visual-only
episodes.

PostgreSQL queues `audio` and `visual` jobs together. A `merge` job is created
exactly once after both branches complete, and every resulting episode receives
its own `memograph` job. That job writes the visual summary and speech transcript
to Memograph as separate concurrent API calls. The visual payload excludes
objects and wrapper labels; the speech payload contains only speaker utterances.
A visual-analysis retry therefore never reruns STT, and a Memograph retry never
reruns either extraction branch.

### Video API

All routes require `Authorization: Bearer <access_token>`.

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/video/recordings` | Upload a complete video |
| `GET` | `/api/v1/video/recordings/:recording_id` | Poll component and episode status |
| `POST` | `/api/v1/video/realtime/sessions` | Start a realtime camera session |
| `POST` | `/api/v1/video/realtime/sessions/:session_id/chunks` | Upload an idempotent video chunk |
| `GET` | `/api/v1/video/realtime/sessions/:session_id` | Read chunk and aggregate progress |
| `POST` | `/api/v1/video/realtime/sessions/:session_id/stop` | Stop the session |

Upload a complete recording:

```bash
curl -X POST http://localhost:8080/api/v1/video/recordings \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@meeting.webm" \
  -F "session_id=meeting-123" \
  -F "group_id=user-456" \
  -F "memory_id=$MEMORY_ID" \
  -F "device_id=browser-camera" \
  -F "location=office"
```

Start a realtime session:

```bash
curl -X POST http://localhost:8080/api/v1/video/realtime/sessions \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "memory_id": "'"$MEMORY_ID"'",
    "group_id": "user-456",
    "device_id": "browser-camera",
    "location": "office",
    "chunk_duration_seconds": 30,
    "frame_interval_seconds": 5
  }'
```

Upload a self-contained MP4/WebM chunk:

```bash
curl -X POST \
  http://localhost:8080/api/v1/video/realtime/sessions/$VIDEO_SESSION_ID/chunks \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@chunk.webm" \
  -F "chunk_id=$(uuidgen | tr '[:upper:]' '[:lower:]')" \
  -F "is_final=false"
```

The client generates only the stable UUID `chunk_id`. The backend atomically
assigns `chunk_index` and `start_time`. Retrying the same file with the same
`chunk_id` returns the existing recording without creating duplicate jobs.
Use a fresh `MediaRecorder` for every browser chunk so each WebM contains its
own container header and can be decoded independently.

In Postman, choose **Body → form-data**, set `file` to type **File**, and add
`chunk_id` and `is_final` as text fields. Do not manually set
`Content-Type`; Postman supplies the multipart boundary. Poll the returned
recording ID until `status` is `completed` or `failed`.

## Authentication

Users authenticate with an email address and password. Email addresses are
normalized to lowercase, passwords are stored as bcrypt hashes, and successful
signup/login requests return an expiring HS256 JWT. Send that token as
`Authorization: Bearer <access_token>` when calling protected endpoints.

```bash
curl -X POST http://localhost:8080/api/v1/auth/signup \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}'

curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}'

curl http://localhost:8080/api/v1/secure \
  -H 'Authorization: Bearer <access_token>'
```

Signup returns HTTP 201, login returns HTTP 200, and the secure test endpoint
returns the authenticated user ID. Missing, invalid, or expired tokens return
HTTP 401.

## Configuration

Configuration is read from `APP_*` environment variables through `GetEnv`,
`GetEnvInt`, and `GetEnvBool`. Non-secret operational settings have fallback
values. `APP_DATABASE_URL` and `APP_JWT_SECRET` are required; the JWT secret must
contain at least 32 characters. `APP_JWT_ACCESS_TOKEN_TTL` controls access-token
lifetime and defaults to `24h`. Copy `.env.example` for local development, but
do not commit `.env`.

Development logging is colored and console-friendly by default. Production
logging defaults to structured JSON. `APP_LOG_PRETTY` can explicitly override
either behavior, and `NO_COLOR=1` disables ANSI colors.

```bash
cp .env.example .env
```

The server automatically loads `.env` when it is present. Variables already
set by the shell or container take precedence.

Voice, video, and Memograph essentials:

```bash
APP_STT_PROVIDER=openai
APP_STT_API_KEY=...
APP_STT_MODEL=gpt-4o-transcribe-diarize

APP_VISION_PROVIDER=openai
APP_VISION_API_KEY=...
APP_VISION_MODEL=gpt-4.1-mini

APP_MEMOGRAPH_BASE_URL=https://your-memograph-host
APP_MEMOGRAPH_API_KEY=mg_live_...
APP_MEMOGRAPH_JWT=...
APP_MEMOGRAPH_TIMEOUT=3m
```

See `.env.example` for storage limits, frame interval, episode duration, worker
concurrency, retry count, provider URLs, and timeouts. `OPENAI_API_KEY` is
accepted as an alias for both `APP_STT_API_KEY` and `APP_VISION_API_KEY`. When
both features use OpenAI, `APP_VISION_API_KEY` may also be omitted and the
backend will reuse `APP_STT_API_KEY`.
Container deployments should mount persistent storage at `/data`. Local
non-container development requires `ffmpeg` on `PATH`; the production image
installs it. The server validates `APP_VIDEO_FFMPEG_PATH` during startup so a
missing executable is reported before any uploads are accepted.

## Database and server

Install sqlc v1.30.0 and Goose v3.25.0, then run:

```bash
make generate
make migrate-up DATABASE_URL="$APP_DATABASE_URL"
make run
```

Migration `00003_voice_memory.sql` adds recordings, episodes, and durable jobs.
Migration `00004_realtime_voice_sessions.sql` adds persistent listening sessions
and idempotent ordered chunks.
Migration `00005_video_memory.sql` adds video sessions, recordings, component
jobs, visual observations, and merged episodes. Local media under `data/` is
ignored by Git.

Example response:

```json
{
  "data": {
    "status": "ok",
    "database": "up",
    "checked_at": "2026-07-22T12:00:00Z"
  },
  "error": "",
  "code": "",
  "message": "service is healthy",
  "paging": null
}
```

If PostgreSQL is unavailable, the endpoint returns HTTP 503 with code
`SERVICE_UNAVAILABLE`.

## Verification

```bash
make test
make vet
make build
```

The test suite covers container dependency contracts, configuration fallbacks,
authentication, episode bucketing and offsets, graph-configuration validation,
OpenAI diarized and visual structured-output parsing, backend-assigned video
chunk ordering, Memograph auth selection, custom fields, and answer scoping.
