package auth

import (
	"context"
	"github.com/122440367/easy-webdav/internal/store"
)

type basicContextKey struct{}

func contextWithUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, basicContextKey{}, u)
}
func UserFromContext(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(basicContextKey{}).(store.User)
	return u, ok
}
