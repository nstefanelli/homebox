-- +goose Up
ALTER TABLE entities ADD COLUMN icon varchar(255) NULL;
