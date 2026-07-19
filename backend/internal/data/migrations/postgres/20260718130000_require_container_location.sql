-- +goose Up
UPDATE entity_types
SET is_location = true
WHERE is_container = true
  AND is_location = false;

ALTER TABLE entity_types
    ADD CONSTRAINT entity_types_container_requires_location
    CHECK (NOT is_container OR is_location);

-- +goose Down
ALTER TABLE entity_types
    DROP CONSTRAINT IF EXISTS entity_types_container_requires_location;
