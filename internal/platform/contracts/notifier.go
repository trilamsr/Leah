package contracts

import "context"

type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}
