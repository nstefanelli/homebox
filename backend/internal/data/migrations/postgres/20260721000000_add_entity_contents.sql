-- +goose Up
ALTER TABLE entities ADD COLUMN contents text NULL;
