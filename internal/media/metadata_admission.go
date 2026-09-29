package media

import (
	"context"
	"database/sql"
)

// MetadataAuthorization lets the HTTP edge revalidate its caller inside the
// catalog transaction without coupling media to accounts or the HTTP server.
type MetadataAuthorization func(*sql.Tx) error

func (c *Catalog) metadataTx(ctx context.Context, authorize []MetadataAuthorization, apply func(*sql.Tx) error) error {
	return c.db.WithTx(ctx, func(tx *sql.Tx) error {
		for _, check := range authorize {
			if check != nil {
				if err := check(tx); err != nil {
					return err
				}
			}
		}
		return apply(tx)
	})
}
