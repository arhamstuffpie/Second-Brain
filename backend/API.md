# Backend API contract

This is the frontend integration contract implemented by the Go backend. The
default local base URL is `http://localhost:8181`; all application endpoints
except `/health` live below `/api/v1`.

## Route inventory

| Method | Path | Auth | Purpose |
|---|---|---:|---|
| `GET` | `/health` | no | Database readiness |
| `POST` | `/api/v1/auth/signup` | no | Create account and token |
| `POST` | `/api/v1/auth/login` | no | Authenticate and create token |
| `GET` | `/api/v1/secure` | yes | Verify token |
| `POST` | `/api/v1/voice/recordings` | yes | Queue audio recording |
| `POST` | `/api/v1/voice/chunks` | yes | Queue audio through the non-realtime chunk alias |
| `GET` | `/api/v1/voice/recordings/:recording_id` | yes | Read audio processing result |
| `POST` | `/api/v1/voice/realtime/sessions` | yes | Start realtime audio capture |
| `POST` | `/api/v1/voice/realtime/sessions/:session_id/chunks` | yes | Queue ordered realtime audio chunk |
| `GET` | `/api/v1/voice/realtime/sessions/:session_id` | yes | Read realtime audio progress |
| `POST` | `/api/v1/voice/realtime/sessions/:session_id/stop` | yes | Stop realtime audio capture |
| `POST` | `/api/v1/voice/projects/:project_id/memories` | yes | Create graph memory |
| `POST` | `/api/v1/voice/memories/:memory_id/search` | yes | Search graph memory |
| `POST` | `/api/v1/voice/memories/:memory_id/answer` | yes | Answer using graph memory |
| `GET` | `/api/v1/voice/memories/:memory_id/graph` | yes | Read graph |
| `POST` | `/api/v1/video/recordings` | yes | Queue video recording |
| `GET` | `/api/v1/video/recordings/:recording_id` | yes | Read video processing result |
| `POST` | `/api/v1/video/realtime/sessions` | yes | Start realtime video capture |
| `POST` | `/api/v1/video/realtime/sessions/:session_id/chunks` | yes | Queue idempotent realtime video chunk |
| `GET` | `/api/v1/video/realtime/sessions/:session_id` | yes | Read realtime video progress |
| `POST` | `/api/v1/video/realtime/sessions/:session_id/stop` | yes | Stop realtime video capture |

## Common behavior

### Authentication

All endpoints except `/health`, `/api/v1/auth/signup`, and
`/api/v1/auth/login` require:

```http
Authorization: Bearer <access_token>
```

The access token is returned by signup and login. A missing token returns
`401` with message `a bearer token is required`. An invalid or expired token
returns `401` with message `the bearer token is invalid or expired`.

### Response envelope

Every JSON response uses this envelope. None of these top-level properties are
omitted, so the frontend can use one response type for every endpoint.   

```ts
type ApiResponse<T> = {
  data: T | null;
  error: string;       // Empty on success; HTTP status text on failure.
  code: string;        // Empty on success; stable machine-readable error code.
  message: string;     // Human-readable result or error detail.
  paging: Paging | null;
};

type Paging = {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
};
```

`paging` is currently always `null`; none of the registered endpoints uses the
paging helper yet.

Success example:

```json
{
  "data": {"user_id": "user-uuid"},
  "error": "",
  "code": "",
  "message": "authenticated request successful",
  "paging": null
}
```

Error example:

```json
{
  "data": null,
  "error": "Bad Request",
  "code": "VALIDATION_ERROR",
  "message": "email: must be a valid email address",
  "paging": null
}
```

### Common status codes

| Status | `code` | Meaning |
|---|---|---|
| `400` | `VALIDATION_ERROR` | Malformed input or a field failed validation. |
| `401` | `UNAUTHORIZED` | Bearer token is missing/invalid, or login credentials are wrong. |
| `403` | `FORBIDDEN` | The authenticated caller is not allowed to perform the operation. |
| `404` | `NOT_FOUND` | An owned resource was not found. |
| `404` | `ROUTE_NOT_FOUND` | No registered route matches the URL. |
| `405` | `METHOD_NOT_ALLOWED` | The URL exists but not for that HTTP method. |
| `409` | `CONFLICT` | The request conflicts with current state, such as uploading to a stopped session. |
| `413` | `UPLOAD_TOO_LARGE` | A media upload exceeds its configured limit. |
| `500` | `INTERNAL_ERROR` | An unexpected backend error occurred. |
| `503` | `SERVICE_UNAVAILABLE` | A required dependency such as PostgreSQL or Memograph is unavailable. |

All responses include an `X-Request-ID` header. The frontend may send its own
`X-Request-ID` (maximum 128 characters); otherwise the backend creates one.
Keep this value in client logs when reporting an API failure.

An allowed CORS preflight (`OPTIONS`) is the one response with no JSON body: it
returns `204 No Content`. A preflight from a disallowed origin returns the
normal JSON error envelope with `403 CORS_ORIGIN_FORBIDDEN`.

Times are JSON-encoded Go `time.Time` values in RFC 3339 format, for example
`"2026-07-27T12:34:56.123456Z"`. Fields described as optional may be absent
because their Go response tag uses `omitempty`.

### Asynchronous media processing

Voice and video uploads return `202 Accepted` after storing the file and
queuing work. This does **not** mean transcription, visual analysis, or memory
insertion is finished. Save `data.id` and poll the corresponding
`GET .../recordings/:recording_id` endpoint. For realtime capture, the session
status endpoint provides chunk-level progress.

## Shared response types

### `AuthResult`

```ts
type AuthResult = {
  user: {
    id: string;
    email: string;
    created_at: string;
    updated_at: string;
  };
  access_token: string;
  token_type: "Bearer";
  expires_at: string;
};
```

### Voice types

```ts
type VoiceRecording = {
  id: string;
  session_id: string;
  group_id: string;
  memory_id: string;
  status:
    | "queued"
    | "transcribing"
    | "memograph_pending"
    | "completed"
    | "failed";
  file_name: string;
  media_type: string;
  size_bytes: number;
  chunk_index?: number;
  is_final?: boolean; // Omitted when false.
  created_at: string;
};

type TranscriptSegment = {
  start_time: number;
  end_time: number;
  speaker: string;
  text: string;
  confidence?: number;
};

type Transcript = {
  text: string;
  language?: string;
  duration: number;
  segments: TranscriptSegment[];
  audio_track_present?: boolean;
  warning?: string;
};

type VoiceEpisode = {
  id: string;
  bucket_index: number;
  start_time: number;
  end_time: number;
  description: string;
  confidence?: number;
  status: "queued" | "writing" | "completed" | "failed";
  memograph_response?: unknown;
  last_error?: string;
};

type VoiceRecordingDetail = VoiceRecording & {
  device_id?: string;
  location?: string;
  transcript?: Transcript;
  episodes: VoiceEpisode[];
  last_error?: string;
  updated_at: string;
};

type RealtimeProgress = {
  total: number;
  queued: number;
  processing: number;
  completed: number;
  failed: number;
  latest_chunk_index: number; // -1 until the first chunk exists.
};

type RealtimeVoiceSession = {
  id: string;
  memory_id: string;
  group_id: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds: number;
  status: "active" | "stopped";
  created_at: string;
  updated_at: string;
  stopped_at?: string;
};

type RealtimeVoiceSessionDetail = RealtimeVoiceSession & {
  progress: RealtimeProgress;
  chunks: VoiceRecording[];
};
```

### Video types

```ts
type VideoRecording = {
  id: string;
  session_id: string;
  group_id: string;
  memory_id: string;
  status:
    | "queued"
    | "processing"
    | "merging"
    | "memograph_pending"
    | "completed"
    | "failed";
  audio_status: "queued" | "processing" | "completed" | "failed";
  visual_status: "queued" | "processing" | "completed" | "failed";
  merge_status: "waiting" | "queued" | "processing" | "completed" | "failed";
  file_name: string;
  media_type: string;
  size_bytes: number;
  chunk_id?: string;
  chunk_index?: number;
  start_time: number;
  is_final?: boolean; // Omitted when false.
  created_at: string;
};

type DetectedObject = {
  name: string;
  confidence?: number;
};

type DetectedText = {
  text: string;
  confidence?: number;
};

type VideoObservation = {
  start_time: number;
  end_time: number;
  objects: DetectedObject[];
  text_detected: DetectedText[];
  activity: string;
  location_guess: string;
  summary: string;
  confidence?: number;
};

type VisualAnalysis = {
  observations: VideoObservation[];
};

type VideoEpisode = {
  id: string;
  bucket_index: number;
  start_time: number;
  end_time: number;
  description: string;
  visual_description?: string;
  speech_description?: string;
  location?: string;
  confidence?: number;
  status: "queued" | "writing" | "completed" | "failed";
  memograph_response?: unknown;
  last_error?: string;
};

type VideoRecordingDetail = VideoRecording & {
  device_id?: string;
  location?: string;
  stt_provider?: string;
  stt_model?: string;
  visual_provider?: string;
  visual_model?: string;
  transcript?: Transcript;
  visual_analysis?: VisualAnalysis;
  episodes: VideoEpisode[];
  last_error?: string;
  updated_at: string;
};

type RealtimeVideoSession = {
  id: string;
  memory_id: string;
  group_id: string;
  device_id?: string;
  location?: string;
  chunk_duration_seconds: number;
  frame_interval_seconds: number;
  next_chunk_index: number;
  status: "active" | "stopped";
  created_at: string;
  updated_at: string;
  stopped_at?: string;
};

type RealtimeVideoSessionDetail = RealtimeVideoSession & {
  progress: RealtimeProgress;
  chunks: VideoRecording[];
};
```

## Health

### `GET /health`

Public database readiness check.

- Parameters: none.
- Success: `200`, message `service is healthy`.
- `data`:

```ts
{
  status: "ok";
  database: "up";
  checked_at: string;
}
```

- Errors: `503 SERVICE_UNAVAILABLE` if the database ping fails.

## Authentication

### `POST /api/v1/auth/signup`

Creates a user, hashes the password, and immediately returns an access token.

Request:

```http
Content-Type: application/json
```

```ts
{
  email: string;    // Required, valid address, normalized to lowercase, max 254 chars.
  password: string; // Required, 8 to 72 bytes.
}
```

- Success: `201`, message `account created`, `data: AuthResult`.
- Errors: `400 VALIDATION_ERROR`, `409 CONFLICT` if the email already exists,
  `503 SERVICE_UNAVAILABLE` for a user-database failure.

### `POST /api/v1/auth/login`

Verifies an email/password pair and returns a new access token.

Request body and validation are identical to signup.

- Success: `200`, message `login successful`, `data: AuthResult`.
- Errors: `400 VALIDATION_ERROR`, `401 UNAUTHORIZED` for an unknown email or
  wrong password, `503 SERVICE_UNAVAILABLE` for a user-database failure.

### `GET /api/v1/secure`

Minimal endpoint for verifying that the stored token still authenticates.

- Authentication: required.
- Parameters: none.
- Success: `200`, message `authenticated request successful`.
- `data`: `{ "user_id": "<subject from access token>" }`.

## Voice recording

### `POST /api/v1/voice/recordings`

Uploads an audio recording and queues speech-to-text and Memograph processing.

Request:

```http
Content-Type: multipart/form-data
Authorization: Bearer <access_token>
```

| Form field | Required | Type | Rules/default |
|---|---:|---|---|
| `file` | yes | file | FLAC, MP3, MP4, MPEG/MPGA, M4A, OGG, WAV, or WebM. An `audio/*` media type is also accepted. Default maximum is 25 MiB and is configurable. |
| `session_id` | yes | string | Client-defined recording/capture session identifier. |
| `memory_id` | yes | string | Memograph memory that receives generated episodes. |
| `group_id` | no | string | Defaults to `session_id`. |
| `device_id` | no | string | Client/device metadata. |
| `location` | no | string | Location metadata. |
| `start_time` | no | number string | Offset in seconds, default `0`, must be non-negative. |
| `confidence` | no | number string | Default confidence attached to generated episodes, from `0` to `1`. |

- Success: `202`, message `audio accepted for processing`,
  `data: VoiceRecording`.
- Errors: `400 VALIDATION_ERROR`, `413 UPLOAD_TOO_LARGE`.
- Frontend action: poll
  `GET /api/v1/voice/recordings/{data.id}` until `status` is `completed` or
  `failed`.

### `POST /api/v1/voice/chunks`

This route currently uses the **same fields and processing behavior** as
`POST /voice/recordings`. Its only observable difference is the success
message `audio chunk accepted for processing`.

It does not currently read `chunk_index` or `is_final`, and it does not provide
chunk idempotency. Use the realtime session/chunk API below when ordered,
retry-safe chunks are required.

- Success: `202`, `data: VoiceRecording`.
- Errors: `400 VALIDATION_ERROR`, `413 UPLOAD_TOO_LARGE`.

### `GET /api/v1/voice/recordings/:recording_id`

Returns an owned recording and its processing results.

- Path `recording_id`: required recording UUID returned by an upload.
- Success: `200`, message `voice recording status`,
  `data: VoiceRecordingDetail`.
- Errors: `404 NOT_FOUND` when the recording does not belong to the caller or
  does not exist.

The frontend should treat `status` as terminal only when it is `completed` or
`failed`. `episodes` is always an array; it is empty before episodes are made.
`transcript` is absent until speech-to-text finishes.

## Realtime voice

### `POST /api/v1/voice/realtime/sessions`

Creates an active realtime voice session.

Request:

```ts
{
  memory_id: string;              // Required.
  group_id?: string;              // Defaults to the generated session id.
  device_id?: string;
  location?: string;
  chunk_duration_seconds?: number; // Default 30; allowed 5..300.
}
```

- Success: `201`, message `realtime voice session started`,
  `data: RealtimeVoiceSession`.
- Errors: `400 VALIDATION_ERROR`.

Keep `data.id`; it is the `session_id` used by all subsequent realtime voice
calls.

### `POST /api/v1/voice/realtime/sessions/:session_id/chunks`

Uploads one ordered audio chunk. Retrying the same `chunk_index` for the same
authenticated user and session returns the already-created recording, making
the operation idempotent by `(owner, session_id, chunk_index)`.

| Input | Required | Type | Rules/default |
|---|---:|---|---|
| Path `session_id` | yes | string | ID returned when the realtime session was created. |
| Form `file` | yes | file | Same formats and size limit as voice recording upload. |
| Form `chunk_index` | yes | integer string | Zero-based and non-negative. |
| Form `is_final` | no | boolean string | Default `false`; accepts values understood by Go boolean parsing. |
| Form `confidence` | no | number string | From `0` to `1`. |

The recording inherits `memory_id`, `group_id`, `device_id`, and `location`
from the session. Its start offset is
`chunk_index * chunk_duration_seconds`. When `is_final=true`, the backend also
stops the session.

- Success: `202`, message `realtime audio chunk accepted`,
  `data: VoiceRecording`.
- Errors: `400 VALIDATION_ERROR`, `404 NOT_FOUND` for an unknown/unowned
  session, `409 CONFLICT` if the session is stopped, `413 UPLOAD_TOO_LARGE`.

### `GET /api/v1/voice/realtime/sessions/:session_id`

- Path `session_id`: required.
- Success: `200`, message `realtime voice session status`,
  `data: RealtimeVoiceSessionDetail`.
- Errors: `404 NOT_FOUND`.

`chunks` is ordered by `chunk_index`. `progress.processing` combines
`transcribing` and `memograph_pending` chunks.

### `POST /api/v1/voice/realtime/sessions/:session_id/stop`

Stops a session without uploading a final chunk. The operation is idempotent:
calling it again leaves `stopped_at` unchanged, though `updated_at` is updated.

- Path `session_id`: required.
- Body: none.
- Success: `200`, message `realtime voice session stopped`,
  `data: RealtimeVoiceSession` with `status: "stopped"`.
- Errors: `404 NOT_FOUND`.

## Graph memory proxy

These authenticated endpoints proxy Memograph. Their success `data` is the
valid JSON body returned by the configured Memograph server, so its inner shape
is upstream-defined and is not transformed by this backend. An empty upstream
body becomes `{}`.

### `POST /api/v1/voice/projects/:project_id/memories`

Creates a graph memory.

- Path `project_id`: required Memograph project ID.
- JSON body:

```ts
type MemoryCreateRequest = {
  name: string;                  // Required.
  memory_type?: string;          // Accepted but always overwritten to "graph".
  embedding_model?: string;      // Default "text-embedding-3-small".
  secret_id?: string;
  custom_fields?: Array<{
    name: string;
    type: string;
    description?: string;
    required?: boolean;
  }>;
  graph_config:
    | {
        mode: "template";
        template: string;        // Required in template mode.
      }
    | {
        mode: "instruction";
        instruction: string;     // Required in instruction mode.
      }
    | {
        mode: "custom";
        entity_types: Record<string, string>; // Non-empty name -> description.
        edge_types: Record<string, string>;   // Non-empty name -> description.
        entity_type_colors?: Record<string, `#${string}`>; // Must be #RRGGBB.
        edge_type_map?: Record<string, string[]>;
      };
};
```

If no custom field named `confidence` is supplied (case-insensitive), the
backend appends:

```json
{
  "name": "confidence",
  "type": "float",
  "description": "Speech-to-text confidence"
}
```

- Success: `201`, message `graph memory created`, `data: unknown` (Memograph
  JSON).
- Errors: `400 VALIDATION_ERROR`, `503 SERVICE_UNAVAILABLE`.

### `POST /api/v1/voice/memories/:memory_id/search`

Searches a graph memory.

```ts
{
  query: string;                   // Required, non-empty.
  limit?: number;                  // Defaults to 10 when <= 0.
  group_id?: string;
  filters?: Record<string, unknown>;
}
```

- Path `memory_id`: required.
- Success: `200`, message `memory search complete`, `data: unknown`
  (Memograph JSON).
- Errors: `400 VALIDATION_ERROR`, `503 SERVICE_UNAVAILABLE`.

### `POST /api/v1/voice/memories/:memory_id/answer`

Generates an answer from graph memory context.

```ts
{
  query?: string;
  messages?: Array<{
    role: string;
    content: string;
  }>;
  limit?: number;                  // Defaults to 10 when <= 0.
  model?: string;
  group_id?: string;
  filters?: Record<string, unknown>;
}
```

At least one of a non-empty `query` or a non-empty `messages` array is
required. When `group_id` is supplied, the backend also combines it into the
upstream filters as a `group_id == value` constraint.

- Path `memory_id`: required.
- Success: `200`, message `memory answer complete`, `data: unknown`
  (Memograph JSON).
- Errors: `400 VALIDATION_ERROR`, `503 SERVICE_UNAVAILABLE`.

### `GET /api/v1/voice/memories/:memory_id/graph`

Returns the graph representation from Memograph.

- Path `memory_id`: required.
- Query `group_id`: optional group restriction.
- Body: none.
- Success: `200`, message `memory graph loaded`, `data: unknown` (Memograph
  JSON).
- Errors: `400 VALIDATION_ERROR`, `503 SERVICE_UNAVAILABLE`.

## Video recording

### `POST /api/v1/video/recordings`

Uploads a video and queues audio extraction/transcription, visual analysis,
merging, and Memograph processing.

| Form field | Required | Type | Rules/default |
|---|---:|---|---|
| `file` | yes | file | MP4, WebM, MOV, M4V, or MKV. A `video/*` media type is also accepted. Default maximum is 250 MiB and is configurable. |
| `session_id` | yes | string | Client-defined capture session identifier. |
| `memory_id` | yes | string | Memograph memory that receives generated episodes. |
| `group_id` | no | string | Defaults to `session_id`. |
| `device_id` | no | string | Client/device metadata. |
| `location` | no | string | Location metadata. |
| `start_time` | no | number string | Offset in seconds, default `0`, must be non-negative. |
| `confidence` | no | number string | Default confidence from `0` to `1`. |

- Success: `202`, message `video accepted for processing`,
  `data: VideoRecording`.
- Errors: `400 VALIDATION_ERROR`, `413 UPLOAD_TOO_LARGE`.
- Frontend action: poll
  `GET /api/v1/video/recordings/{data.id}` until `status` is `completed` or
  `failed`.

### `GET /api/v1/video/recordings/:recording_id`

- Path `recording_id`: required recording UUID returned by an upload.
- Success: `200`, message `video recording status`,
  `data: VideoRecordingDetail`.
- Errors: `404 NOT_FOUND`.

The status fields let the UI display each pipeline stage independently.
`episodes` is always an array. `transcript` and `visual_analysis` are absent
until their respective stages finish. A video without an audio track still
gets a transcript object with `audio_track_present: false`, an empty
`segments` array, and a warning.

## Realtime video

### `POST /api/v1/video/realtime/sessions`

Creates an active realtime video session.

```ts
{
  memory_id: string;               // Required.
  group_id?: string;               // Defaults to generated session id.
  device_id?: string;
  location?: string;
  chunk_duration_seconds?: number; // Default 30; allowed 5..300.
  frame_interval_seconds?: number; // Config default (normally 5); allowed
                                   // 1..60 and <= chunk_duration_seconds.
}
```

- Success: `201`, message `realtime video session started`,
  `data: RealtimeVideoSession`.
- Errors: `400 VALIDATION_ERROR`.

Keep `data.id`; it is the path `session_id` for subsequent calls.

### `POST /api/v1/video/realtime/sessions/:session_id/chunks`

Uploads a video chunk. The client supplies a stable UUID `chunk_id`; the server
assigns the ordered numeric `chunk_index`. Retrying the same `chunk_id` for the
same caller/session returns the existing recording instead of creating a
duplicate.

| Input | Required | Type | Rules/default |
|---|---:|---|---|
| Path `session_id` | yes | string | ID returned when the session was created. |
| Form `file` | yes | file | Same formats and size limit as video recording upload. |
| Form `chunk_id` | yes | UUID string | Client-generated idempotency key. Normalized to lowercase. |
| Form `is_final` | no | boolean string | Default `false`. |
| Form `confidence` | no | number string | From `0` to `1`. |

The recording inherits session metadata. Its `start_time` is the assigned
`chunk_index * chunk_duration_seconds`. When `is_final=true`, the backend also
stops the session.

- Success: `202`, message `realtime video chunk accepted`,
  `data: VideoRecording`.
- Errors: `400 VALIDATION_ERROR`, `404 NOT_FOUND`, `409 CONFLICT` if the
  session is stopped, `413 UPLOAD_TOO_LARGE`.

### `GET /api/v1/video/realtime/sessions/:session_id`

- Path `session_id`: required.
- Success: `200`, message `realtime video session status`,
  `data: RealtimeVideoSessionDetail`.
- Errors: `404 NOT_FOUND`.

`chunks` is ordered by server-assigned `chunk_index`.
`progress.processing` combines recordings in `processing`, `merging`, and
`memograph_pending`.

### `POST /api/v1/video/realtime/sessions/:session_id/stop`

Stops a video session without uploading a final chunk. The operation is
idempotent.

- Path `session_id`: required.
- Body: none.
- Success: `200`, message `realtime video session stopped`,
  `data: RealtimeVideoSession` with `status: "stopped"`.
- Errors: `404 NOT_FOUND`.

## Minimal frontend flow

1. Signup or login and persist `data.access_token` plus `data.expires_at`.
2. Send `Authorization: Bearer <token>` for every `/api/v1` feature call.
3. Create/select a graph memory and keep its `memory_id`.
4. Upload media. On `202`, save the returned recording `id`.
5. Poll the matching recording endpoint with a modest delay until the status
   is terminal; render intermediate status rather than treating `202` as done.
6. For realtime capture, create a session first, upload chunks with stable
   indexes/UUIDs, then send a final chunk or call `/stop`.
7. On failures, branch on the stable envelope `code`, show `message` where
   appropriate, and log the response `X-Request-ID`.
