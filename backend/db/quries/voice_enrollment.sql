-- name: CreateVoiceEnrollmentSample :one
WITH available_slot AS (
    SELECT slot
    FROM generate_series(0, 3) AS slot
    WHERE NOT EXISTS (
        SELECT 1 FROM voice_enrollment_samples
        WHERE owner_user_id = $1 AND voice_enrollment_samples.slot = slot
    )
    ORDER BY slot
    LIMIT 1
)
INSERT INTO voice_enrollment_samples (
    owner_user_id, slot, provider_label, file_name, file_path,
    media_type, size_bytes, duration_seconds
)
SELECT $1, slot, $2, $3, $4, $5, $6, $7
FROM available_slot
ON CONFLICT (owner_user_id, slot) DO NOTHING
RETURNING id, owner_user_id, slot, provider_label, file_name, file_path,
          media_type, size_bytes, duration_seconds, created_at, updated_at;

-- name: ListVoiceEnrollmentSamples :many
SELECT id, owner_user_id, slot, provider_label, file_name, file_path,
       media_type, size_bytes, duration_seconds, created_at, updated_at
FROM voice_enrollment_samples
WHERE owner_user_id = $1
ORDER BY slot;

-- name: GetVoiceEnrollmentSample :one
SELECT id, owner_user_id, slot, provider_label, file_name, file_path,
       media_type, size_bytes, duration_seconds, created_at, updated_at
FROM voice_enrollment_samples
WHERE id = $1 AND owner_user_id = $2;

-- name: DeleteVoiceEnrollmentSample :one
DELETE FROM voice_enrollment_samples
WHERE id = $1 AND owner_user_id = $2
RETURNING id, owner_user_id, slot, provider_label, file_name, file_path,
          media_type, size_bytes, duration_seconds, created_at, updated_at;
