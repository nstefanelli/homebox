-- +goose Up
ALTER TABLE entity_types ADD COLUMN is_container boolean NOT NULL DEFAULT false;
