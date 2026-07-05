-- +goose Up
ALTER TABLE groups ADD COLUMN integrations JSON DEFAULT '{}';
