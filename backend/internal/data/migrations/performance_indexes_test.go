package migrations_test

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/migrations"
	_ "github.com/sysadminsmedia/homebox/backend/internal/data/migrations/sqlite3"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	_ "github.com/sysadminsmedia/homebox/backend/pkgs/cgofreesqlite"
)

const (
	tenantIndexMigration                = "20260718000000_add_tenant_query_indexes.sql"
	tenantIndexPreviousMigrationVersion = int64(20260705130000)
	tenantIndexTestGroupID              = "group-a"
)

var tenantQueryIndexes = []string{
	"idx_entities_group_archived_name",
	"idx_entities_group_parent",
	"idx_tags_group_name",
	"idx_tags_group_parent",
	"idx_entity_types_group_name",
	"idx_entity_templates_group_name",
	"idx_user_groups_group_user",
	"idx_tag_entities_entity_tag",
	"idx_template_fields_template",
	"idx_entity_fields_entity",
}

func TestTenantQueryIndexMigrationsStayInDialectParity(t *testing.T) {
	t.Parallel()

	for _, dialect := range []string{config.DriverSqlite3, config.DriverPostgres} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()

			migrationFS, err := migrations.Migrations(dialect)
			require.NoError(t, err)
			raw, err := migrationFS.ReadFile(dialect + "/" + tenantIndexMigration)
			require.NoError(t, err)
			normalized := strings.ToLower(strings.ReplaceAll(string(raw), `"`, ""))
			createPrefix := "create index if not exists "
			dropPrefix := "drop index if exists "
			if dialect == config.DriverPostgres {
				assert.Contains(t, normalized, "-- +goose no transaction")
				createPrefix = "create index concurrently if not exists "
				dropPrefix = "drop index concurrently if exists "
			}

			for _, indexName := range tenantQueryIndexes {
				assert.Contains(t, normalized, createPrefix+indexName)
				assert.Contains(t, normalized, dropPrefix+indexName)
			}
		})
	}
}

func TestSQLiteTenantQueryIndexesImproveDominantPlans(t *testing.T) {
	db := openMigratedSQLite(t)
	_, err := db.Exec("PRAGMA automatic_index = OFF")
	require.NoError(t, err)

	for _, indexName := range tenantQueryIndexes {
		_, err := db.Exec("DROP INDEX IF EXISTS " + indexName)
		require.NoError(t, err)
	}

	type planCase struct {
		name          string
		query         string
		args          []any
		expectedIndex string
	}
	cases := []planCase{
		{
			name:          "default entity list and tenant-scoped search candidate set",
			query:         "SELECT id, name FROM entities WHERE group_entities = ? AND archived = 0 ORDER BY name LIMIT 100",
			args:          []any{tenantIndexTestGroupID},
			expectedIndex: "idx_entities_group_archived_name",
		},
		{
			name:          "entity hierarchy traversal",
			query:         "SELECT id FROM entities WHERE group_entities = ? AND entity_children = ?",
			args:          []any{tenantIndexTestGroupID, "parent-a"},
			expectedIndex: "idx_entities_group_parent",
		},
		{
			name:          "tag list",
			query:         "SELECT id, name FROM tags WHERE group_tags = ? ORDER BY name",
			args:          []any{tenantIndexTestGroupID},
			expectedIndex: "idx_tags_group_name",
		},
		{
			name:          "tag hierarchy traversal",
			query:         "SELECT id FROM tags WHERE group_tags = ? AND tag_children = ?",
			args:          []any{tenantIndexTestGroupID, "parent-a"},
			expectedIndex: "idx_tags_group_parent",
		},
		{
			name:          "entity type list",
			query:         "SELECT id, name FROM entity_types WHERE group_entity_types = ? ORDER BY name",
			args:          []any{tenantIndexTestGroupID},
			expectedIndex: "idx_entity_types_group_name",
		},
		{
			name:          "entity template list",
			query:         "SELECT id, name FROM entity_templates WHERE group_entity_templates = ? ORDER BY name",
			args:          []any{tenantIndexTestGroupID},
			expectedIndex: "idx_entity_templates_group_name",
		},
		{
			name:          "group member traversal",
			query:         "SELECT user_id FROM user_groups WHERE group_id = ?",
			args:          []any{tenantIndexTestGroupID},
			expectedIndex: "idx_user_groups_group_user",
		},
		{
			name:          "entity tag eager load",
			query:         "SELECT tag_id FROM tag_entities WHERE entity_id = ?",
			args:          []any{"entity-a"},
			expectedIndex: "idx_tag_entities_entity_tag",
		},
		{
			name:          "template field eager load",
			query:         "SELECT id FROM template_fields WHERE entity_template_fields = ?",
			args:          []any{"template-a"},
			expectedIndex: "idx_template_fields_template",
		},
		{
			name:          "entity field eager load",
			query:         "SELECT id FROM entity_fields WHERE entity_fields = ?",
			args:          []any{"entity-a"},
			expectedIndex: "idx_entity_fields_entity",
		},
	}

	before := make(map[string]string, len(cases))
	for _, tc := range cases {
		before[tc.name] = sqliteQueryPlan(t, db, tc.query, tc.args...)
		assert.NotContains(t, before[tc.name], tc.expectedIndex)
	}

	applySQLiteTenantIndexMigration(t, db)

	for _, tc := range cases {
		after := sqliteQueryPlan(t, db, tc.query, tc.args...)
		t.Logf("%s\nbefore: %s\nafter:  %s", tc.name, before[tc.name], after)
		assert.Contains(t, after, tc.expectedIndex)
		assert.NotEqual(t, before[tc.name], after)
	}

	require.NoError(t, goose.DownTo(
		db,
		config.DriverSqlite3,
		tenantIndexPreviousMigrationVersion,
	))
	for _, indexName := range tenantQueryIndexes {
		assert.False(t, sqliteIndexExists(t, db, indexName), "down migration must drop %s", indexName)
	}
}

func openMigratedSQLite(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1&_time_format=sqlite")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	migrationFS, err := migrations.Migrations(config.DriverSqlite3)
	require.NoError(t, err)
	goose.SetBaseFS(migrationFS)
	require.NoError(t, goose.SetDialect(config.DriverSqlite3))
	require.NoError(t, goose.Up(db, config.DriverSqlite3))
	return db
}

func applySQLiteTenantIndexMigration(t *testing.T, db *sql.DB) {
	t.Helper()

	migrationFS, err := migrations.Migrations(config.DriverSqlite3)
	require.NoError(t, err)
	raw, err := migrationFS.ReadFile(config.DriverSqlite3 + "/" + tenantIndexMigration)
	require.NoError(t, err)

	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok)
	up = strings.TrimPrefix(up, "-- +goose Up")
	_, err = db.Exec(up)
	require.NoError(t, err)
}

func sqliteQueryPlan(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()

	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	sort.Strings(details)
	return fmt.Sprintf("%s", details)
}

func sqliteIndexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()

	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
		indexName,
	).Scan(&count)
	require.NoError(t, err)
	return count == 1
}
