# Multimodal pipeline gaps and implementation tracker

This document explains what is missing, why it matters, and how each change
will improve the system. A checked item is implemented. An unchecked item is
still required before the new pipeline can be trusted in production.

Rollout note: migration 21 adds dependency storage without queuing historical
recordings. New and explicitly reprocessed recordings receive the seven-stage
run. Old recordings stay unchanged until controlled backfill is implemented.

## Simple end-to-end picture

```text
Video
  -> audio analysis + dense face tracking
  -> transcription
  -> identity matching
  -> active-speaker fusion
  -> episode generation
  -> graph persistence
  -> activate the completed version
```

The application should keep using the previous complete version while this
work is running.

## The four main improvements

### 1. Complete analysis run

Status: **Run structure implemented; remaining stage workers are not yet all wired.**

Why we need it:

- Previously, a run could contain only dense face tracking.
- Face, voice, identity, episode, and graph work could finish independently
  without one shared definition of “complete.”
- A failed stage was difficult to understand from the run record.

What is implemented:

- Every new or reprocessed video receives one run with seven required stages.
- Each stage stores the stages it depends on.
- Dense tracking can run immediately.
- Later stages stay incomplete until their real workers complete them.
- Existing runs are deliberately left unchanged to avoid automatic backfill.

```text
audio_analysis ------------------------> transcription
dense_person_tracking ----------------> identity_matching
transcription ------------------------> identity_matching
identity_matching --------------------> active_speaker_fusion
active_speaker_fusion ----------------> episode_generation
episode_generation -------------------> graph_persistence
```

Example:

> Face tracking succeeds, but audio analysis fails.

The run remains incomplete and shows exactly which stage failed. Successful
face work does not need to be repeated when audio processing is retried.

How it improves the system:

- One clear status for the complete recording analysis.
- Independent retries instead of restarting everything.
- No false “completed” state after only one model finishes.
- Easier debugging and safer version activation later.

Still needed:

- Connect the audio, transcription, dense identity, active-speaker, episode,
  and graph workers to their corresponding stage rows.

### 2. Use dense tracks for identity matching

Status: **Not implemented.**

Why we need it:

- Dense tracking follows every visible face many times per second.
- The current identity path can still use a few sampled frames and its older
  temporary face tracks.
- Two people crossing or appearing together can therefore be missed or mixed.

Example:

> Arham and John cross in front of the camera.

Dense tracking keeps two physical track IDs. Identity matching should use the
best gallery embeddings from each track, instead of treating occasional scene
frames as the main identity evidence.

How it improves the system:

- Fewer missed faces.
- Fewer duplicate people.
- Lower risk of attaching one person’s name to another person.
- Better recovery after a face is briefly hidden.

Implementation outline:

- Match scene timestamps and face boxes to dense observations.
- Match each dense track’s gallery embeddings to owner-scoped face profiles.
- Keep ambiguous matches unknown.
- Remove `localFaceTrack` only after comparison tests pass.

### 3. Activate completed versions

Status: **Database activation function exists; safe coordinator call is not implemented.**

Why we need it:

- Users should see one complete version, not a mixture of old and new results.
- A new run can fail while the previous version is still valid.

Example:

```text
Version 2: complete and visible
Version 3: face complete, audio still processing
```

Version 2 should stay active until every required version 3 stage is complete.
Then the database should switch versions in one transaction.

How it improves the system:

- No half-finished result becomes visible.
- The previous version remains a safe rollback.
- Readers always use a consistent set of evidence.

Implementation outline:

- Check that every required stage is completed.
- Call `activate_analysis_run` once.
- Test that incomplete and dead runs cannot activate.
- Test that a complete run activates atomically.

### 4. Controlled backfill

Status: **Not implemented. Migration 20 still queues all existing recordings.**

Why we need it:

- A production database may contain thousands of old recordings.
- Sending all of them to a CPU analyzer creates a long queue and keeps the
  machine busy continuously.
- New recordings may wait behind old work.

Example:

```text
Unsafe: 2,000 old recordings -> one large queue
Safe:       5 recordings -> check health -> next 5
```

How it improves the system:

- Predictable CPU and memory use.
- New recordings can receive higher priority.
- Backfill can be paused after repeated failures.
- Safer rollout and easier capacity planning.

Implementation outline:

- Keep schema migrations separate from operational backfill.
- Add a batch command with recording, memory, date, and limit filters.
- Enforce a maximum number of queued backfill jobs.
- Start with the two newest test recordings, then increase slowly.

## Are the additional gaps real?

### Dense tracks exist, but identity may still use sampled faces

**Yes.** Dense results are stored, while the visual identity path still calls
the sampled face tracker. Using dense tracks will improve identity accuracy in
multi-person and moving-camera videos.

### Face observations do not mean a person is identified

**Yes.** A face observation means “a face was visible here.” It does not mean
the system knows the person’s name. Identification still requires a compatible
owner-scoped face profile, a strong similarity score, and a clear margin over
the second-best match. This separation prevents confident-looking false names.

### All stages were not coordinated inside one run

**Yes.** The durable seven-stage run structure is now implemented. The gap is
partially closed: the remaining workers must update those stage rows before the
run can finish.

### Completed runs may not become active automatically

**Yes.** The database function exists, but no safe coordinator calls it after
all required stages finish. Adding that coordinator will make version changes
consistent and reversible.

### Dead jobs need manual requeue

**Yes.** Retryable jobs restart automatically; dead jobs intentionally stop.
This is safe because a permanently broken file should not loop forever. A
validated admin requeue action would improve recovery after fixing a model,
configuration, or temporary infrastructure problem.

### Old recordings need controlled backfill

**Yes.** Migration-driven full backfill is simple but not safe at large scale.
Small batches will protect normal uploads and make failures easier to stop.

### Active-speaker fusion must connect the correct face and voice

**Yes.** Face tracking answers “who is visible,” while diarization answers
“which voice spoke.” Fusion must use matching time ranges and mouth activity to
connect them. This improves speaker names in transcripts and prevents a silent
listener from being labeled as the speaker.

### End-to-end tests are still needed

**Yes.** Unit tests prove individual pieces, but they do not prove the complete
chain. Production tests should cover:

```text
two faces detected
  -> two stable tracks
  -> correct face profiles
  -> correct voice linked to the speaking track
  -> every required stage completed
  -> new version activated
```

These tests reduce regressions where every component works alone but their IDs,
timestamps, or versions do not line up.

## Recommended implementation order

- [x] Create the complete seven-stage run and dependency graph.
- [ ] Replace migration-driven full backfill with controlled batches.
- [ ] Connect the overlap-audio worker and transcription stage.
- [ ] Map scene people to dense tracks.
- [ ] Match dense tracks to face profiles.
- [ ] Fuse the correct dense face track with the correct voice.
- [ ] Complete episode and graph stage reporting.
- [ ] Activate only fully completed runs.
- [ ] Add dead-job inspection and manual requeue.
- [ ] Add detection-to-activation end-to-end tests.

## Short design decisions

- **Reuse the existing job table:** fewer moving parts and one retry model.
- **Store dependencies on each job:** the run is understandable from the
  database without a second workflow service.
- **Do not activate yet:** unfinished required workers must not produce a false
  complete version.
- **Keep uncertain identity unknown:** a missed name is safer than a wrong name.
