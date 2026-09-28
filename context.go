package monogo

import (
	"context"
)

type contextKey struct{}
type contextMap map[string]interface{}

func WithContext(ctx context.Context, fields map[string]interface{}) context.Context {
	if len(fields) == 0 {
		return ctx
	}

	existing := FromContext(ctx)
	merged := make(contextMap, len(existing)+len(fields))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}

	return context.WithValue(ctx, contextKey{}, merged)
}

func WithField(ctx context.Context, key string, val interface{}) context.Context {
	return WithContext(ctx, map[string]interface{}{key: val})
}

func FromContext(ctx context.Context) map[string]interface{} {
	if ctx == nil {
		return nil
	}
	if val, ok := ctx.Value(contextKey{}).(contextMap); ok {
		cp := make(map[string]interface{}, len(val))
		for k, v := range val {
			cp[k] = v
		}
		return cp
	}
	return nil
}
