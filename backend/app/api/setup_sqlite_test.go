package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteDSNWithImmediateTransactions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "plain path",
			dsn:  "homebox.db",
			want: "homebox.db?_txlock=immediate",
		},
		{
			name: "existing query",
			dsn:  "homebox.db?_pragma=busy_timeout=1000&_fk=1",
			want: "homebox.db?_pragma=busy_timeout=1000&_fk=1&_txlock=immediate",
		},
		{
			name: "trailing separator",
			dsn:  "homebox.db?",
			want: "homebox.db?_txlock=immediate",
		},
		{
			name: "explicit deferred mode is preserved",
			dsn:  "homebox.db?_txlock=deferred&_fk=1",
			want: "homebox.db?_txlock=deferred&_fk=1",
		},
		{
			name: "case-insensitive operator mode is preserved",
			dsn:  "homebox.db?_TXLOCK=exclusive",
			want: "homebox.db?_TXLOCK=exclusive",
		},
		{
			name: "fragment remains last",
			dsn:  "homebox.db?_fk=1#fragment",
			want: "homebox.db?_fk=1&_txlock=immediate#fragment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, sqliteDSNWithImmediateTransactions(tt.dsn))
		})
	}
}

func TestSQLiteImmediateTransactionsSerializeWriters(t *testing.T) {
	dsn := sqliteDSNWithImmediateTransactions(fmt.Sprintf(
		"file:%s?_pragma=busy_timeout=1000&_pragma=journal_mode=WAL",
		filepath.Join(t.TempDir(), "writer-lock.db"),
	))
	db, err := sql.Open("sqlite3", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(2)

	_, err = db.Exec(`
		CREATE TABLE writer_lock_test (id INTEGER PRIMARY KEY, value TEXT);
		INSERT INTO writer_lock_test (id, value) VALUES (1, 'one'), (2, 'two');
	`)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	first, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	var value string
	require.NoError(t, first.QueryRowContext(ctx, "SELECT value FROM writer_lock_test WHERE id = 1").Scan(&value))

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		second, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			secondDone <- beginErr
			return
		}
		if _, execErr := second.ExecContext(ctx, "DELETE FROM writer_lock_test WHERE id = 2"); execErr != nil {
			_ = second.Rollback()
			secondDone <- execErr
			return
		}
		secondDone <- second.Commit()
	}()

	<-secondStarted
	select {
	case earlyErr := <-secondDone:
		t.Fatalf("second writer completed before the first transaction released its lock: %v", earlyErr)
	case <-time.After(50 * time.Millisecond):
	}

	_, err = first.ExecContext(ctx, "DELETE FROM writer_lock_test WHERE id = 1")
	require.NoError(t, err)
	require.NoError(t, first.Commit())
	require.NoError(t, <-secondDone)

	var remaining int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM writer_lock_test").Scan(&remaining))
	assert.Zero(t, remaining)
}
