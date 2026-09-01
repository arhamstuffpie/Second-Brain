# Multi-person pipeline progress

This file tracks the replacement plan. A checked item is implemented in code; rollout items stay unchecked until measured on real recordings.

## Implemented

- [x] Processing version 3 storage contract.
- [x] Storage schema for durable analysis runs and per-stage states: queued, processing, completed, retryable failed, and dead.
- [x] Atomic database function for switching the active completed version.
- [x] Dense `POST /v1/person-tracks` inference contract.
- [x] Full-video face detection at a configurable 8 FPS default.
- [x] Concurrent Kalman tracks with two-pass motion/appearance association, confirmation, loss, expiry, and short re-identification.
- [x] Deterministic track and observation IDs.
- [x] Per-observation boxes, landmarks, quality, pose, embeddings, mouth visibility, and mouth activity.
- [x] Time-and-pose gallery selection with unusable samples excluded.
- [x] Overlap-aware `POST /v1/audio-analysis` inference contract.
- [x] Mono 16 kHz normalization, regular pyannote overlap timeline, silence boundaries, and stable region/source IDs.
- [x] CPU-budgeted SepFormer separation with explicit ambiguous and budget-exhausted outcomes.
- [x] Offline model loading, checksums, bearer auth, bounded uploads/temp storage, non-root containers, `/healthz`, and `/readyz`.
- [x] Additive transcript, word-attribution, person-track, audio-region, audio-source, and identity-evidence schemas.
- [x] Backend HTTP clients validate service provenance and reject malformed track/source relationships.
- [x] Durable dense-person worker with independent claim, stale-lock recovery, retry backoff, and dead-letter state.
- [x] Atomic persistence for dense person tracks, face observations, stage checkpoints, and model provenance.
- [x] Owner-scoped storage for curated gallery embeddings from every confirmed face track.
- [x] Seven-stage analysis-run manifest with explicit stage dependencies.
- [x] Versioned identity worker matching dense track galleries to owner-scoped face profiles.
- [x] Conservative timestamp-and-face-box mapping from sampled scenes to dense tracks.
- [x] Automatic identity linking and automatic profile merging remain disabled.

## Still required before production enablement

- [ ] Connect the overlap-audio client to its independently retryable backend worker and persist its checkpoints/results.
- [ ] Make every remaining worker complete its matching analysis-stage row.
- [ ] Store separated WAV outputs as owner-scoped `evidence_review` media assets, then transcribe accepted sources.
- [ ] Match separated sources to non-overlap ECAPA cluster centroids; keep failures ambiguous.
- [ ] Remove the sampled `localFaceTrack` legacy fallback after shadow comparison passes.
- [ ] Fuse mouth activity, temporal coverage, face match, and voice match into candidate evidence.
- [ ] Add stage metrics, trace propagation, kill switches, and the synchronized admin evidence view.
- [ ] Add a CUDA image/profile with the same API contracts.
- [ ] Run the labeled evaluation corpus and document thresholds, IDF1/HOTA, DER, SI-SDR, WER, false-link rate, CPU time, memory, and disk.
- [ ] Shadow rollout, review-only identity links, throttled backfill, and rollback exercise.

## Short design decisions

- **Separate ML services:** Python runs models; Go keeps retries, ownership, and identity decisions. This avoids two sources of truth.
- **Version 3 instead of overwrite:** old evidence stays usable until the new run completes.
- **Dense tracking stays separate from scene sampling:** faces need 8 FPS; scene descriptions do not.
- **Use the existing stage queue:** one durable retry system is easier to operate than another custom queue.
- **Persist stage dependencies:** blocked work is visible without adding another workflow service.
- **Store gallery embeddings only:** a few strong samples per face are enough for matching and keep the database smaller.
- **Kalman + two-pass association:** motion handles nearby frames; SFace helps after crossing or occlusion.
- **Unknown is a valid result:** weak separation or matching stays ambiguous instead of creating a false identity.
- **Embeddings only cross the private ML contract:** they are never logged or added to public download APIs.
- **Model files are prepared before startup:** production does not depend on internet access.
- **One CPU inference worker:** bounded work is slower but avoids memory spikes and queue collapse.
- **Additive JSON fields:** old clients can keep decoding the existing scalar speaker fields.
- **No automatic merge yet:** one wrong biometric merge costs more than a missed suggestion.
