package utils

import "context"

type principalContextKey struct{}
type memographAPIKeyContextKey struct{}

type Principal struct {
	Subject string
	Issuer  string
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// WithMemographAPIKey attaches an optional per-request Memograph credential.
// The user JWT is still required by the HTTP middleware; this value is only
// forwarded to Memograph and never establishes an application principal.
func WithMemographAPIKey(ctx context.Context, apiKey string) context.Context {
	return context.WithValue(ctx, memographAPIKeyContextKey{}, apiKey)
}

func MemographAPIKeyFromContext(ctx context.Context) string {
	apiKey, _ := ctx.Value(memographAPIKeyContextKey{}).(string)
	return apiKey
}
