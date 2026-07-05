-- +goose Up
ALTER TABLE groups ADD COLUMN integrations JSONB DEFAULT '{}';
