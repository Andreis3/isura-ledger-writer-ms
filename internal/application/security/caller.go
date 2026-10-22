// Package security carries the authenticated platform principal into application commands.
package security

import "context"

const ResolveFundsHold = "funds_hold.resolve"

// Principal is populated only by the trusted platform authentication boundary.
type Principal struct {
	Subject     string
	Permissions []string
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	principal.Permissions = append([]string(nil), principal.Permissions...)
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	principal.Permissions = append([]string(nil), principal.Permissions...)
	return principal, ok
}

func (p Principal) Allows(permission string) bool {
	for _, candidate := range p.Permissions {
		if candidate == permission {
			return true
		}
	}
	return false
}
