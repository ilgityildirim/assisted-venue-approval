package specs

import (
	"context"
)

// Specification defines the Specification pattern for domain objects.
// It supports composition via And/Or/Not and evaluation with context for cancellation and timeouts.
// Keep implementations small and focused; compose for complexity.
// Generic to allow reuse across domain types.

type Specification[T any] interface {
	IsSatisfiedBy(ctx context.Context, v T) bool
	And(other Specification[T]) Specification[T]
	Or(other Specification[T]) Specification[T]
	Not() Specification[T]
}

type specFunc[T any] func(ctx context.Context, v T) bool

func (f specFunc[T]) IsSatisfiedBy(ctx context.Context, v T) bool { return f(ctx, v) }

func (f specFunc[T]) And(other Specification[T]) Specification[T] {
	return specFunc[T](func(ctx context.Context, v T) bool {
		if ctx.Err() != nil { // cancelled or timed out
			return false
		}
		if !f(ctx, v) {
			return false
		}
		return other.IsSatisfiedBy(ctx, v)
	})
}

func (f specFunc[T]) Or(other Specification[T]) Specification[T] {
	return specFunc[T](func(ctx context.Context, v T) bool {
		if ctx.Err() != nil {
			return false
		}
		if f(ctx, v) {
			return true
		}
		return other.IsSatisfiedBy(ctx, v)
	})
}

func (f specFunc[T]) Not() Specification[T] {
	return specFunc[T](func(ctx context.Context, v T) bool {
		if ctx.Err() != nil {
			return false
		}
		return !f(ctx, v)
	})
}

// New constructs a Specification from a predicate.
func New[T any](fn func(ctx context.Context, v T) bool) Specification[T] { return specFunc[T](fn) }
