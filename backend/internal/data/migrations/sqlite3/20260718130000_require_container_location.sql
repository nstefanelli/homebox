-- +goose Up
UPDATE entity_types
SET is_location = true
WHERE is_container = true
  AND is_location = false;

ALTER TABLE entity_types
    ADD COLUMN container_location_guard INTEGER NOT NULL DEFAULT 1
    CONSTRAINT entity_types_container_requires_location
    CHECK (NOT is_container OR is_location);

-- +goose Down
ALTER TABLE entity_types DROP COLUMN container_location_guard;
