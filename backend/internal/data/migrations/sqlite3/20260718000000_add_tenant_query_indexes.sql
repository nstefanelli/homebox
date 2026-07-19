-- +goose Up
-- The dominant collection views always scope by group before filtering or
-- ordering. These composite indexes keep that work inside one tenant and also
-- cover hierarchy traversal, reverse many-to-many lookups, and template field
-- eager loading.
CREATE INDEX IF NOT EXISTS idx_entities_group_archived_name
    ON entities(group_entities, archived, name);
CREATE INDEX IF NOT EXISTS idx_entities_group_parent
    ON entities(group_entities, entity_children);

CREATE INDEX IF NOT EXISTS idx_tags_group_name
    ON tags(group_tags, name);
CREATE INDEX IF NOT EXISTS idx_tags_group_parent
    ON tags(group_tags, tag_children);

CREATE INDEX IF NOT EXISTS idx_entity_types_group_name
    ON entity_types(group_entity_types, name);
CREATE INDEX IF NOT EXISTS idx_entity_templates_group_name
    ON entity_templates(group_entity_templates, name);

-- user_groups has a (user_id, group_id) primary key, and tag_entities has a
-- (tag_id, entity_id) primary key. Their reverse traversal directions need
-- explicit indexes for group-member and entity-tag reads.
CREATE INDEX IF NOT EXISTS idx_user_groups_group_user
    ON user_groups(group_id, user_id);
CREATE INDEX IF NOT EXISTS idx_tag_entities_entity_tag
    ON tag_entities(entity_id, tag_id);

CREATE INDEX IF NOT EXISTS idx_template_fields_template
    ON template_fields(entity_template_fields);
CREATE INDEX IF NOT EXISTS idx_entity_fields_entity
    ON entity_fields(entity_fields);

-- +goose Down
DROP INDEX IF EXISTS idx_entity_fields_entity;
DROP INDEX IF EXISTS idx_template_fields_template;
DROP INDEX IF EXISTS idx_tag_entities_entity_tag;
DROP INDEX IF EXISTS idx_user_groups_group_user;
DROP INDEX IF EXISTS idx_entity_templates_group_name;
DROP INDEX IF EXISTS idx_entity_types_group_name;
DROP INDEX IF EXISTS idx_tags_group_parent;
DROP INDEX IF EXISTS idx_tags_group_name;
DROP INDEX IF EXISTS idx_entities_group_parent;
DROP INDEX IF EXISTS idx_entities_group_archived_name;
