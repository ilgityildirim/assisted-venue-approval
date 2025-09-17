// Package errors provides structured error types used across the application.
// We prefer these over raw fmt.Errorf strings to enable reliable checks with
// errors.Is / errors.As and to carry minimal context about the failure.
package errors

import (
	"errors"
	"fmt"
)

// ValidationError indicates invalid input/config/state provided by a caller/user.
// Keep fields minimal; add codes when we have real classification needs.
type ValidationError struct {
	Op  string // where it happened (package.Function)
	Msg string // human friendly message (no PII)
	Err error  // underlying cause (optional)
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("validation: %s: %s: %v", e.Op, e.Msg, e.Err)
	}
	return fmt.Sprintf("validation: %s: %s", e.Op, e.Msg)
}

func (e *ValidationError) Unwrap() error           { return e.Err }
func (e *ValidationError) Operation() string       { return e.Op }
func (e *ValidationError) Message() string         { return e.Msg }
func (e *ValidationError) Context() map[string]any { return map[string]any{"op": e.Op, "msg": e.Msg} }

func NewValidation(op, msg string, err error) error {
	return &ValidationError{Op: op, Msg: msg, Err: err}
}

// DBError represents database access/operation failures.
type DBError struct {
	Op  string
	Msg string
	Err error
}

func (e *DBError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("db: %s: %s: %v", e.Op, e.Msg, e.Err)
	}
	return fmt.Sprintf("db: %s: %s", e.Op, e.Msg)
}

func (e *DBError) Unwrap() error           { return e.Err }
func (e *DBError) Operation() string       { return e.Op }
func (e *DBError) Message() string         { return e.Msg }
func (e *DBError) Context() map[string]any { return map[string]any{"op": e.Op, "msg": e.Msg} }

func NewDB(op, msg string, err error) error { return &DBError{Op: op, Msg: msg, Err: err} }

// ExternalAPIError represents failures in external services (HTTP APIs, SDKs, etc.).
type ExternalAPIError struct {
	Op     string
	Msg    string
	Err    error
	System string // optional system name e.g. "google" / "openai"
}

func (e *ExternalAPIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	sys := e.System
	if sys == "" {
		sys = "external"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %s: %v", sys, e.Op, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s: %s", sys, e.Op, e.Msg)
}

func (e *ExternalAPIError) Unwrap() error     { return e.Err }
func (e *ExternalAPIError) Operation() string { return e.Op }
func (e *ExternalAPIError) Message() string   { return e.Msg }
func (e *ExternalAPIError) Context() map[string]any {
	return map[string]any{"op": e.Op, "msg": e.Msg, "system": e.System}
}

func NewExternal(op, system, msg string, err error) error {
	return &ExternalAPIError{Op: op, System: system, Msg: msg, Err: err}
}

// BizError is for domain/business logic failures that aren't programmer bugs.
type BizError struct {
	Op  string
	Msg string
	Err error
}

func (e *BizError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return fmt.Sprintf("biz: %s: %s: %v", e.Op, e.Msg, e.Err)
	}
	return fmt.Sprintf("biz: %s: %s", e.Op, e.Msg)
}

func (e *BizError) Unwrap() error           { return e.Err }
func (e *BizError) Operation() string       { return e.Op }
func (e *BizError) Message() string         { return e.Msg }
func (e *BizError) Context() map[string]any { return map[string]any{"op": e.Op, "msg": e.Msg} }

func NewBiz(op, msg string, err error) error { return &BizError{Op: op, Msg: msg, Err: err} }

// IsKind helpers: allow callers to check error kind without type assertions.
// Example: if errors.Is(err, errors.ErrValidation) { ... }
var (
	ErrValidation = &ValidationError{}
	ErrDB         = &DBError{}
	ErrExternal   = &ExternalAPIError{}
	ErrBiz        = &BizError{}
)

// Is enables errors.Is(err, ErrValidation) via errors.As semantics.
// We delegate to errors.As with the zero-value pointer of each type.
func Is(err, target error) bool {
	if err == nil || target == nil {
		return errors.Is(err, target)
	}
	switch target.(type) {
	case *ValidationError:
		var v *ValidationError
		return errors.As(err, &v)
	case *DBError:
		var d *DBError
		return errors.As(err, &d)
	case *ExternalAPIError:
		var ex *ExternalAPIError
		return errors.As(err, &ex)
	case *BizError:
		var b *BizError
		return errors.As(err, &b)
	default:
		return errors.Is(err, target)
	}
}
