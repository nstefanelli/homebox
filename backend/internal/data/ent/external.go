package ent

import (
	"context"
	stdsql "database/sql"

	entsql "entgo.io/ent/dialect/sql"
)

// Sql exposes the underlying database connection in the ent client
// so that we can use it to perform custom queries.
func (c *Client) Sql() *stdsql.DB {
	return c.driver.(*entsql.Driver).DB()
}

// RawQueryContext executes a custom query through the client's configured
// driver. Unlike Sql, it also works for clients bound to an ent transaction.
func (c *Client) RawQueryContext(ctx context.Context, query string, args ...any) (*entsql.Rows, error) {
	rows := &entsql.Rows{}
	if err := c.driver.Query(ctx, query, args, rows); err != nil {
		return nil, err
	}
	return rows, nil
}
