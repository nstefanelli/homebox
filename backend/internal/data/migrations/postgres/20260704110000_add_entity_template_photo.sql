-- +goose Up
ALTER TABLE entity_templates ADD COLUMN photo_path varchar(500) NULL;
ALTER TABLE entity_templates ADD COLUMN photo_mime_type varchar(255) NULL;
