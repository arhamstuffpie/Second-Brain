# Server-side face identity setup

## Implemented server components

This repository now includes:

- `face-embedder/`: private YuNet detection/alignment and SFace embedding service;
- `internal/face`: bounded, provider-neutral Go client with health/model validation
  and retryability classification;
- migration `00016_temporal_person_identity.sql`: canonical people, face profiles
  and samples, recording-local person/object tracks, identity-link evidence,
  action events, and an independent temporal job queue;
- authenticated `/api/v1/people` enrollment, recognition, naming, listing, and
  deletion routes; and
- automatic face detection, embedding, account-scoped matching, and canonical
  person IDs in the server-side video-analysis worker; and
- deterministic action and face/voice-link validators with model-free fixtures.

The mobile app remains capture/review only. Raw face vectors are accepted only
inside the Go service/repository boundary and are never present in API responses.

## Decision

Use the following self-hosted CPU models for the first face-identity adapter:

| Task | Model | Purpose |
| --- | --- | --- |
| Face detection and five landmarks | `opencv/face_detection_yunet:face_detection_yunet_2023mar.onnx` | Find a face and the landmarks needed for alignment. |
| Face embedding | `opencv/face_recognition_sface:face_recognition_sface_2021dec.onnx` | Convert the aligned face into a normalized SFace vector. |

YuNet and SFace are small, run through `opencv-python-headless` on CPU, and their
model directories have permissive licenses. YuNet's files are MIT-licensed and
SFace's files are Apache-2.0-licensed:

- <https://github.com/opencv/opencv_zoo/tree/main/models/face_detection_yunet>
- <https://github.com/opencv/opencv_zoo/tree/main/models/face_recognition_sface>

Do **not** use an automatically downloaded InsightFace `buffalo_*` or
`antelopev2` pack as the production default. InsightFace's code is MIT, but its
published pretrained packs are restricted to non-commercial research unless a
separate model license is obtained:
<https://github.com/deepinsight/insightface#license>.

The exact face model ID stored with every vector should be:

```text
opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
```

SFace produces a 128-value feature vector. The service must still discover and
validate that dimension when it loads the model instead of trusting a constant.
Vectors from another model ID or dimension must never be compared with these.

## What runs where

The mobile app only captures media, uploads it, shows status, and presents
review controls. It must never receive or compute a face vector.

```text
mobile upload
    -> Go API: auth, ownership, durable jobs, profile lifecycle
    -> FFmpeg: timestamped server-side frame extraction
    -> face-embedder: detect, align, quality-gate, embed
    -> PostgreSQL: account-scoped matching and centroids
    -> temporal jobs: tracks, active speaker, actions, evidence validation
    -> Memograph: confirmed derived facts only
```

Keep these decisions separate:

| Question | Server component |
| --- | --- |
| Is there a usable face? | YuNet plus deterministic quality gates. |
| Does it match an enrolled person? | SFace embedding plus account-scoped cosine matching in PostgreSQL. |
| Is it the same visible person through this clip? | A recording-local person/face tracker. |
| Was this face speaking? | A separate active-speaker detector around diarized speech. |
| Did this person perform an action? | Temporal person/object tracks plus the action state-transition validator. |

A face match alone must never link a voice, assign an action, or prove physical
presence. Faces on screens, photographs, posters, and ambiguous mirrors must be
excluded before persistent identity matching.

## Private face-embedder contract

Add a sibling service named `face-embedder`, following the existing
`speaker-embedder` layout and security rules. It owns inference only; the Go API
owns people, names, enrollment, matching, consent, and deletion.

`GET /healthz` should return the exact detector and embedder IDs, their SHA-256
hashes, the output dimension, and `status: ok`. Readiness must fail if either
configured file is absent, has the wrong hash, or cannot run a startup probe.

`POST /v1/embeddings` should accept multipart fields:

- `file`: JPEG, PNG, or WebP bytes;
- `model`: the exact SFace model ID above;
- `single_face`: `true` for enrollment and `false` for video frames.

It should return:

```json
{
  "model": "opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79",
  "dimensions": 128,
  "faces": [
    {
      "box": {"x": 121, "y": 64, "width": 190, "height": 190},
      "landmarks": [[171, 121], [258, 119], [217, 163], [178, 208], [252, 207]],
      "detection_score": 0.98,
      "quality": {"usable": true, "reasons": [], "score": 0.91},
      "pose": {"yaw": -48.2, "pitch": 1.4, "roll": 0.8, "bucket": "left_profile"},
      "embedding": [0.012, -0.034]
    }
  ]
}
```

The service must reject oversized or undecodable files, unsupported models,
non-finite/zero vectors, and enrollment images containing zero or multiple
faces. It should L2-normalize every vector. A bounded request timeout and bearer
key must be supported exactly as in `speaker-embedder`.

Quality gating should reject, rather than try to recognize, faces that are too
small, strongly blurred, badly exposed, heavily occluded, or missing reliable
landmarks. Usable turned faces are assigned a coarse pose bucket and retained
in a compact embedding-only gallery; store pose and quality with the evidence.

## Model installation

Production must not download biometric models at process startup. Download and
verify them as an explicit image-build or deployment step:

```bash
mkdir -p face-embedder/models

curl -fL \
  https://huggingface.co/opencv/face_detection_yunet/resolve/main/face_detection_yunet_2023mar.onnx \
  -o face-embedder/models/face_detection_yunet_2023mar.onnx

curl -fL \
  https://huggingface.co/opencv/face_recognition_sface/resolve/main/face_recognition_sface_2021dec.onnx \
  -o face-embedder/models/face_recognition_sface_2021dec.onnx

printf '%s  %s\n' \
  8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4 \
  face-embedder/models/face_detection_yunet_2023mar.onnx \
  | shasum -a 256 -c -

printf '%s  %s\n' \
  0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79 \
  face-embedder/models/face_recognition_sface_2021dec.onnx \
  | shasum -a 256 -c -
```

Pin the runtime as well; a suitable starting dependency set is:

```text
fastapi==0.115.12
numpy==2.2.6
opencv-python-headless==4.11.0.86
python-multipart==0.0.20
uvicorn==0.34.3
```

The repository packages the download and hash verification:

```bash
make face-models
make face-up
curl http://127.0.0.1:8092/healthz
```

`make face-up` fails readiness if either model is absent, has the wrong hash, or
cannot perform the startup inference probe. The container never downloads a
model. `face-embedder/models/*.onnx` is intentionally ignored by Git.

The service can use OpenCV's `FaceDetectorYN` and `FaceRecognizerSF` APIs. The
official detection, alignment, feature, and cosine-matching example is here:
<https://docs.opencv.org/4.11.0/d0/dd4/tutorial_dnn_face.html>.

## Configuration

The private inference container should use:

```dotenv
FACE_EMBEDDER_DETECTOR_PATH=/models/face_detection_yunet_2023mar.onnx
FACE_EMBEDDER_DETECTOR_SHA256=8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4
FACE_EMBEDDER_MODEL_PATH=/models/face_recognition_sface_2021dec.onnx
FACE_EMBEDDER_MODEL=opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
FACE_EMBEDDER_MODEL_SHA256=0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
# Optional only for loopback development; set a managed secret in production.
FACE_EMBEDDER_API_KEY=
FACE_EMBEDDER_MAX_UPLOAD_BYTES=10485760
```

The Go API should use:

```dotenv
APP_FACE_RECOGNITION_PROVIDER=local
APP_FACE_RECOGNITION_BASE_URL=http://127.0.0.1:8092
# Must equal FACE_EMBEDDER_API_KEY when that service key is set.
APP_FACE_RECOGNITION_API_KEY=
APP_FACE_RECOGNITION_MODEL=opencv/sface-2021dec@sha256:0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
APP_FACE_RECOGNITION_TIMEOUT=10s

# Suggestions only until thresholds are calibrated on representative POV video.
APP_FACE_AUTO_CONFIRM=false
APP_FACE_ENROLLMENT_STORAGE_DIR=./data/face-enrollment
APP_FACE_MAX_UPLOAD_MB=10
```

Bind local development to `127.0.0.1:8092`. For a separate host, use a private
network or HTTPS and treat the bearer key as a managed secret.

The API key is not required when both variables are empty and the embedder is
bound to loopback. It is required for production or any non-loopback/private
deployment; configure the same value on both services.

Apply the schema and start the API:

```bash
make migrate-up DATABASE_URL="$APP_DATABASE_URL"
make run
```

The API validates `/healthz` during startup whenever
`APP_FACE_RECOGNITION_PROVIDER` is not `disabled`.

### API smoke test

Enroll a consented face. Reuse `person_profile_id` for additional pose/lighting
samples belonging to the same person:

```bash
curl -X POST http://localhost:8181/api/v1/people/face-enrollments \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@face.jpg" \
  -F "display_name=Arham" \
  -F "relationship_category=other" \
  -F "consent_confirmed=true"
```

Run a server-side comparison without retaining the probe image:

```bash
curl -X POST http://localhost:8181/api/v1/people/face-recognition \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@probe.jpg"
```

Canonical identity management routes are:

```text
GET    /api/v1/people
PATCH  /api/v1/people/:person_profile_id
DELETE /api/v1/people/:person_profile_id
```

When a user has reviewed a recording and explicitly confirmed that one visual
label and one named voice are the same person, link them atomically:

```bash
curl -X POST http://localhost:8181/api/v1/people/identity-links \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "recording_ids": ["recording-id-1", "recording-id-2"],
    "visual_label": "person-1",
    "voice_speaker_profile_id": "speaker-profile-id",
    "confirmed": true
  }'
```

This creates or reuses one canonical person, assigns that ID to the confirmed
visual observations and voice segments, stores accepted manual-link evidence,
and requeues the affected speech and visual graph revisions. It never infers a
link from co-occurrence alone.

The recognition response contains the stable person ID, status, scores, and
quality reasons only. It never contains embeddings or storage paths.

## Enrollment

Enrollment is an authenticated Go API operation, not a direct call from the
mobile app to the inference container:

1. Ask for consent and capture 5--10 guided images with small pose and lighting
   changes. The camera owner needs a separate guided selfie flow because their
   face is normally behind the POV camera.
2. The Go API checks account ownership and sends each image to the private
   service with `single_face=true`.
3. Reject a sample unless exactly one quality-approved live face is present.
   Basic motion prompts can reduce accidental photo enrollment, but production
   anti-spoofing requires a separately evaluated liveness provider/model.
4. Store each normalized vector and a normalized centroid under one
   account-scoped `face_profile`, with exact provider, model, dimension, sample
   count, consent, and retention metadata.
5. Keep only the minimum encrypted review thumbnails required by the product;
   never return vectors or storage paths to the client.

Do not average arbitrary images blindly. Compute the sample-to-centroid cosine
scores and reject an outlier or a mixed-identity enrollment for human review.

## Recognition

The recording worker now runs this path automatically after visual analysis. It
only sends frames containing exactly one visually verified, physically present,
face-visible person to YuNet/SFace. A known match receives the existing
`person_profile_id`. The first unknown frame remains an unresolved recording-local
track. A 30-day, consent-pending provisional profile is created only after a
second consistent, quality-approved frame continues that track; later samples
extend its pose-aware, embedding-only gallery. Ambiguous matches remain track
evidence and do not create another person. Face-service failures add a batch
warning and do not fail speech or visual memory processing.

Unresolved visual tracks are written to Memograph as `VisualOccurrence`
entities with `visual-track:<track-id>` IDs, never as permanent `Person` nodes.
Only a backend `person_profile_id` produces a graph `Person`. Explicitly linking
a named voice to a visual label reuses that visual profile when it already
exists, confirms its face gallery, and requeues affected graph episodes. The
operation fails safely if the selected voice and visual evidence already point
to different canonical people.

Memograph uses `person-profile:<id>` for a resolved face, so the same face keeps
one node across recording sessions. Naming a voice profile does not by itself
name or link a face profile: that link requires independent active-speaker
evidence or explicit review.

Recognition should happen per temporal face track, not per isolated frame:

1. Extract candidate-window frames on the server with exact timestamps.
2. Detect and track faces locally within the recording.
3. Embed several quality-approved, well-spaced frames from the same track.
4. Aggregate only mutually consistent vectors into a normalized track vector.
5. Compare it in PostgreSQL only against profiles with the same owner, exact
   model ID, and dimension.
6. Accept a known-person suggestion only if the top cosine score clears a
   calibrated threshold and beats the runner-up by a calibrated margin.
7. Require repeated agreement across separated frames before creating a
   provisional track identity. Otherwise keep the person unknown or create a
   review suggestion.

The current worker implements this conservatively for sampled frames that have
exactly one visible person. Dense multi-face tracking and a normalized aggregate
track vector require person bounding boxes/track assignments from the visual
pipeline and are not inferred from label order.

## Automatic face and voice resolution

The durable temporal job now sends the original video, resolved face tracks,
and diarized segments to a configured active-speaker service. The service uses
multipart `POST /v1/active-speakers` with `file`, `model`, and JSON `metadata`
fields and returns track/segment evidence with active-speaker score, visible-mouth
coverage, overlap conflicts, and evidence frame IDs.

```dotenv
APP_ACTIVE_SPEAKER_PROVIDER=local
APP_ACTIVE_SPEAKER_BASE_URL=http://127.0.0.1:8093
APP_ACTIVE_SPEAKER_API_KEY=
APP_ACTIVE_SPEAKER_MODEL=active-speaker-v1
APP_ACTIVE_SPEAKER_AUTO_LINK=false
APP_PERSON_AUTO_MERGE=false
APP_ACTIVE_SPEAKER_SCORE_THRESHOLD=0.85
APP_ACTIVE_SPEAKER_MIN_MOUTH_COVERAGE=0.75
APP_ACTIVE_SPEAKER_MIN_TEMPORAL_COVERAGE=0.75
APP_ACTIVE_SPEAKER_MIN_UTTERANCES=2
APP_PERSON_MERGE_EVIDENCE_COUNT=3
```

Automatic linking and merging are deliberately off by default. When enabled,
an accepted face/voice conflict creates a merge candidate. Evidence is counted
once per recording; after the configured number of independent recordings, the
lower-priority profile becomes an alias of the confirmed/named canonical person.
The old profile is archived, not deleted, and affected graph episodes are
requeued. Face samples remain historically attached to the alias, while future
matching resolves them to the canonical person.

OpenCV reports a cosine threshold of `0.363` on LFW for its published SFace
verification example. That is a benchmark reference, **not** a safe production
1:N identification threshold. Keep auto-confirmation disabled until false
acceptance, demographic slices, camera motion, blur, occlusion, and look-alike
cases have been measured on representative POV recordings. Tune the threshold
and runner-up margin from that evaluation, preferring unknown results over false
identity assignments.

The existing speaker-profile SQL pattern can be reused: `DOUBLE PRECISION[]`
centroids, exact model/dimension filters, cosine scoring, an account advisory
lock, provisional profiles, stable observations, and processing-version-aware
idempotency.

## Identity and action safeguards

- Face enrollment/recognition, voice recognition, active-speaker detection, and
  action attribution remain separate provider boundaries.
- A face and ECAPA voice profile can be linked only after temporal overlap,
  visible-mouth coverage, positive active-speaker evidence, no unresolved
  overlapping speaker, adequate face and voice margins, and preferably repeated
  agreement across separated utterances.
- Clothing and body appearance may continue a track inside one recording, but
  must not become cross-session identity evidence.
- First-person hands belong to an unknown camera wearer unless separate evidence
  establishes the owner; never assign a visible bystander's face to those hands.
- An action is emitted only after the deterministic before/during/after state
  transition validates it. A face match does not validate an action.
- PostgreSQL remains authoritative for biometric data and review state.
  Memograph receives only stable IDs and confirmed, timestamped provenance.

## Rollout checks

Before enabling automatic recognition:

- service startup rejects a missing or hash-mismatched model;
- enrollment rejects zero, multiple, blurred, tiny, and spoof-test faces;
- the same image produces the same normalized vector within a pinned runtime;
- different model IDs/dimensions cannot be matched;
- all profile, sample, and observation operations reject cross-account IDs;
- repeated processing does not duplicate observations or graph facts;
- inference failure records a warning and does not rerun STT or block existing
  sparse visual memories;
- deletion removes the face profile, samples, review media, links, and derived
  identity access according to the retention policy;
- a representative evaluation reports false-acceptance rate, false-rejection
  rate, unknown rate, and performance by relevant demographic/recording slices.

This setup covers production-shaped server-side detection, alignment,
enrollment embeddings, account-scoped matching, canonical identity persistence,
and durable active-speaker identity resolution. It does not claim calibrated
POV-video accuracy or liveness. The repository provides the production HTTP
adapter and worker but does not bundle active-speaker model weights; configure
and evaluate a licensed provider before enabling automatic links or merges.
