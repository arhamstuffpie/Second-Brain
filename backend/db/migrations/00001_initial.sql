-- +goose Up
-- The health endpoint has no schema dependencies. This baseline migration gives
-- every environment an explicit starting version for future schema changes.
SELECT 1;

-- +goose Down
SELECT 1;

