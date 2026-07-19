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
	containerConstraintMigration        = "20260718130000_require_container_location.sql"
	containerConstraintMigrationVersion = int64(20260718130000)
	containerConstraintPreviousVersion  = int64(20260718000000)
)

func TestContainerLocationConstraintMigrationsStayInDialectParity(t *testing.T) {
	t.Parallel()

	for _, dialect := range []string{config.DriverSqlite3, config.DriverPostgres} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()

			migrationFS, err := migrations.Migrations(dialect)
			require.NoError(t, err)
			raw, err := migrationFS.ReadFile(dialect + "/" + containerConstraintMigration)
			require.NoError(t, err)
			normalized := strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))

			assert.Contains(t, normalized, "update entity_types set is_location = true")
			assert.Contains(t, normalized, "where is_container = true and is_location = false")
			assert.Contains(t, normalized, "entity_types_container_requires_location")
			assert.Contains(t, normalized, "check (not is_container or is_location)")
		})
	}
}

func TestSQLiteContainerLocationConstraintRepairsAndRejectsInvalidRows(t *testing.T) {
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
	require.NoError(t, goose.UpTo(
		db,
		config.DriverSqlite3,
		containerConstraintPreviousVersion,
	))

	const groupID = "00000000-0000-4000-8000-000000000001"
	_, err = db.Exec(`
		INSERT INTO groups (id, created_at, updated_at, name, currency)
		VALUES (?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'constraint test', 'usd')
	`, groupID)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO entity_types (
			id, created_at, updated_at, name, description,
			is_location, is_container, group_entity_types
		)
		VALUES (
			'00000000-0000-4000-8000-000000000002',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'legacy invalid', '',
			false, true, ?
		)
	`, groupID)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(
		db,
		config.DriverSqlite3,
		containerConstraintMigrationVersion,
	))

	var isLocation, isContainer bool
	err = db.QueryRow(`
		SELECT is_location, is_container
		FROM entity_types
		WHERE id = '00000000-0000-4000-8000-000000000002'
	`).Scan(&isLocation, &isContainer)
	require.NoError(t, err)
	assert.True(t, isLocation, "legacy containers must be normalized into locations")
	assert.True(t, isContainer)

	_, err = db.Exec(`
		INSERT INTO entity_types (
			id, created_at, updated_at, name, description,
			is_location, is_container, group_entity_types
		)
		VALUES (
			'00000000-0000-4000-8000-000000000003',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'invalid insert', '',
			false, true, ?
		)
	`, groupID)
	require.Error(t, err)

	_, err = db.Exec(`
		INSERT INTO entity_types (
			id, created_at, updated_at, name, description,
			is_location, is_container, group_entity_types
		)
		VALUES (
			'00000000-0000-4000-8000-000000000004',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'item', '',
			false, false, ?
		)
	`, groupID)
	require.NoError(t, err)
	_, err = db.Exec(`
		UPDATE entity_types
		SET is_container = true
		WHERE id = '00000000-0000-4000-8000-000000000004'
	`)
	require.Error(t, err)

	_, err = db.Exec(`
		INSERT INTO entity_types (
			id, created_at, updated_at, name, description,
			is_location, is_container, group_entity_types
		)
		VALUES (
			'00000000-0000-4000-8000-000000000005',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'valid container', '',
			true, true, ?
		)
	`, groupID)
	require.NoError(t, err)
}
