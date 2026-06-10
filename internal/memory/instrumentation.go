package memory

import (
	"context"
	"fmt"

	"github.com/trilam/leah/internal/obs"
)

func EmitQuery(registry *obs.Registry, table string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_memory_queries_total").Inc(map[string]string{"table": table})
}

type SelfChecker struct{ Store *Store }

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	if c == nil || c.Store == nil {
		return nil
	}
	rows, err := c.Store.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		if v != "ok" {
			return fmt.Errorf("sqlite integrity: %s", v)
		}
	}
	return rows.Err()
}
