package sqliteopen

import (
	"fmt"
	"net/url"
)

func RODSN(path string) string {
	return fmt.Sprintf(
		"file:%s?mode=ro&immutable=1&_journal_mode=OFF&_query_only=1",
		url.PathEscape(path),
	)
}
