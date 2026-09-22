package utils

import (
	"context"
)

func NewContextWithValue(ctx context.Context, key string, value interface{}) context.Context {
	ctx = context.WithValue(ctx, key, value)
	return ctx
}

// CloneContext creates a copy of the context by collecting all values first,
// then creating a new context with these values in a single layer.
func CloneContext(ctx context.Context) context.Context {
	values := make(map[any]any)

	// Collect all values from the context chain
	var collectValues func(context.Context)
	collectValues = func(c context.Context) {
		if c == nil {
			return
		}

		if vc, ok := c.(interface{ Value(key any) any }); ok {
			for _, k := range []any{
				"",          // string
				0,           // int
				struct{}{},  // empty struct
				(*int)(nil), // pointer
			} {
				if v := vc.Value(k); v != nil && v != c {
					values[k] = v
				}
			}
		}

		// Try to get parent context
		if p, ok := c.Value(struct{}{}).(context.Context); ok && p != c {
			collectValues(p)
		}
	}

	collectValues(ctx)

	// Create a new context with all values in a single layer
	newCtx := context.Background()
	for k, v := range values {
		if v != nil {
			//nolint:fatcontext
			newCtx = context.WithValue(newCtx, k, v)
		}
	}

	return newCtx
}
