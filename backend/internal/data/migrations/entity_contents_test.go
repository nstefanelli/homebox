package migrations_test

import (
	"database/sql"
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
	entityContentsMigration        = "20260721000000_add_entity_contents.sql"
	entityContentsMigrationVersion = int64(20260721000000)
	entityContentsPreviousVersion  = int64(20260718130000)
)

func TestEntityContentsMigrationsStayInDialectParity(t *testing.T) {
	t.Parallel()

	for _, dialect := range []string{config.DriverSqlite3, config.DriverPostgres} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()

			migrationFS, err := migrations.Migrations(dialect)
			require.NoError(t, err)
			raw, err := migrationFS.ReadFile(dialect + "/" + entityContentsMigration)
			require.NoError(t, err)
			normalized := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))

			// Additive nullable column only: old binaries must keep working
			// against the migrated schema.
			assert.Contains(t, normalized, "alter table entities add column contents text null")
			assert.NotContains(t, normalized, "not null")
			assert.NotContains(t, normalized, "drop")
			assert.NotContains(t, normalized, "default")
		})
	}
}

func TestSQLiteEntityContentsMigrationIsAdditiveAndPreservesRows(t *testing.T) {
	db, err := sql.Open(
		"sqlite3",
		"file:"+t.Name()+"?mode=memory&cache=shared&_fk=1&_time_format=sqlite",
	)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	migrationFS, err := migrations.Migrations(config.DriverSqlite3)
	require.NoError(t, err)
	goose.SetBaseFS(migrationFS)
	require.NoError(t, goose.SetDialect(config.DriverSqlite3))
	require.NoError(t, goose.UpTo(db, config.DriverSqlite3, entityContentsPreviousVersion))

	const (
		groupID  = "00000000-0000-4000-8000-000000000001"
		typeID   = "00000000-0000-4000-8000-000000000002"
		entityID = "00000000-0000-4000-8000-000000000003"
	)
	_, err = db.Exec(`
		INSERT INTO groups (id, created_at, updated_at, name, currency)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'contents test', 'usd')
	`, groupID)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO entity_types (
			id, created_at, updated_at, name, description,
			is_location, is_container, group_entity_types
		)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'tote', '', true, true, ?)
	`, typeID, groupID)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO entities (
			id, created_at, updated_at, name,
			group_entities, entity_type_entities
		)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'pre-migration tote', ?, ?)
	`, entityID, groupID, typeID)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(db, config.DriverSqlite3, entityContentsMigrationVersion))

	// The column exists and is nullable with no default.
	var name, colType string
	var notNull int
	var dflt sql.NullString
	err = db.QueryRow(`
		SELECT name, type, "notnull", dflt_value
		FROM pragma_table_info('entities')
		WHERE name = 'contents'
	`).Scan(&name, &colType, &notNull, &dflt)
	require.NoError(t, err, "entities.contents column must exist after the migration")
	assert.Equal(t, "text", strings.ToLower(colType))
	assert.Zero(t, notNull, "contents must be nullable")
	assert.False(t, dflt.Valid, "contents must have no default")

	// The pre-migration row survives untouched, with NULL contents.
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM entities`).Scan(&count))
	assert.Equal(t, 1, count)
	var contents sql.NullString
	require.NoError(t, db.QueryRow(
		`SELECT contents FROM entities WHERE id = ?`, entityID,
	).Scan(&contents))
	assert.False(t, contents.Valid, "existing rows must keep NULL contents")

	// Old-binary tolerance: an INSERT that does not mention the new column
	// (what a pre-upgrade binary issues) must still succeed.
	_, err = db.Exec(`
		INSERT INTO entities (
			id, created_at, updated_at, name,
			group_entities, entity_type_entities
		)
		VALUES (
			'00000000-0000-4000-8000-000000000004',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'old binary insert', ?, ?
		)
	`, groupID, typeID)
	require.NoError(t, err)

	// And the new column round-trips text with embedded newlines.
	manifest := "Baby Hats\n3x AA batteries"
	_, err = db.Exec(`UPDATE entities SET contents = ? WHERE id = ?`, manifest, entityID)
	require.NoError(t, err)
	var stored string
	require.NoError(t, db.QueryRow(
		`SELECT contents FROM entities WHERE id = ?`, entityID,
	).Scan(&stored))
	assert.Equal(t, manifest, stored)
}
