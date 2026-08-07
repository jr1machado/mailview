// Package tenant establishes the only permitted database boundary for
// tenant-scoped operations. SET LOCAL is deliberately transaction-bound so a
// pooled PostgreSQL connection can never leak a prior request's tenant.
package tenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

var ErrMissingContext = errors.New("tenant context is required")

type Context struct {
	TenantID  uuid.UUID
	UserID    int
	RequestID string
}

type contextKey struct{}

func WithContext(ctx context.Context, value Context) context.Context {
	return context.WithValue(ctx, contextKey{}, value)
}

func FromContext(ctx context.Context) (Context, bool) {
	value, ok := ctx.Value(contextKey{}).(Context)
	if !ok || value.TenantID == uuid.Nil {
		return Context{}, false
	}
	return value, true
}

// Begin opens a transaction and binds all PostgreSQL policy variables with
// set_config(..., true), the parameterized equivalent of SET LOCAL.
func Begin(ctx context.Context, db *sqlx.DB) (*sqlx.Tx, Context, error) {
	value, ok := FromContext(ctx)
	if !ok {
		return nil, Context{}, ErrMissingContext
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, Context{}, err
	}
	if _, err := tx.ExecContext(ctx, `
SELECT set_config('app.tenant_id', $1, true),
       set_config('app.user_id', $2, true),
       set_config('app.request_id', $3, true)`,
		value.TenantID.String(), fmt.Sprintf("%d", value.UserID), value.RequestID); err != nil {
		tx.Rollback()
		return nil, Context{}, err
	}
	return tx, value, nil
}

func InTransaction(ctx context.Context, db *sqlx.DB, fn func(*sqlx.Tx, Context) error) error {
	tx, value, err := Begin(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx, value); err != nil {
		return err
	}
	return tx.Commit()
}
