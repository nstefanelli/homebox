-- +goose NO TRANSACTION
-- PostgreSQL builds these online so an upgrade does not hold a write-blocking
-- table lock while indexing an established collection.

-- +goose Up
-- Keep the highest-volume collection reads tenant-local and support their
-- default ordering, hierarchy traversal, reverse many-to-many lookups, and
-- eager-loaded custom fields.
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_entities_group_archived_name"
    ON "entities" ("group_entities", "archived", "name");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_entities_group_parent"
    ON "entities" ("group_entities", "entity_children");

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_tags_group_name"
    ON "tags" ("group_tags", "name");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_tags_group_parent"
    ON "tags" ("group_tags", "tag_children");

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_entity_types_group_name"
    ON "entity_types" ("group_entity_types", "name");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_entity_templates_group_name"
    ON "entity_templates" ("group_entity_templates", "name");

-- The primary keys lead with user_id and tag_id respectively, so PostgreSQL
-- cannot use them efficiently for the reverse traversal direction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_user_groups_group_user"
    ON "user_groups" ("group_id", "user_id");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_tag_entities_entity_tag"
    ON "tag_entities" ("entity_id", "tag_id");

CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_template_fields_template"
    ON "template_fields" ("entity_template_fields");
CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_entity_fields_entity"
    ON "entity_fields" ("entity_fields");

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS "idx_entity_fields_entity";
DROP INDEX CONCURRENTLY IF EXISTS "idx_template_fields_template";
DROP INDEX CONCURRENTLY IF EXISTS "idx_tag_entities_entity_tag";
DROP INDEX CONCURRENTLY IF EXISTS "idx_user_groups_group_user";
DROP INDEX CONCURRENTLY IF EXISTS "idx_entity_templates_group_name";
DROP INDEX CONCURRENTLY IF EXISTS "idx_entity_types_group_name";
DROP INDEX CONCURRENTLY IF EXISTS "idx_tags_group_parent";
DROP INDEX CONCURRENTLY IF EXISTS "idx_tags_group_name";
DROP INDEX CONCURRENTLY IF EXISTS "idx_entities_group_parent";
DROP INDEX CONCURRENTLY IF EXISTS "idx_entities_group_archived_name";
