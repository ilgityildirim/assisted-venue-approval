package domain

import "context"

// UnitOfWork coordinates a set of repository operations within a single
// database transaction to ensure consistency across multiple entities.
// It also exposes repository capabilities so services can operate through it.
//
// Typical usage:
//  uow, err := factory.Begin(ctx)
//  if err != nil { ... }
//  defer uow.Rollback()
//  if err := uow.UpdateVenueActiveCtx(ctx, id, status); err != nil { ... }
//  if err := uow.SaveValidationResultCtx(ctx, vr); err != nil { ... }
//  if err := uow.Commit(); err != nil { ... }
//
// NOTE: Keep the transaction as short as possible.
// TODO: consider adding a helper CommitOrRollback pattern.
//
//go:generate mockgen -destination=../../mocks/mock_uow.go -package=mocks assisted-venue-approval/internal/domain UnitOfWork,UnitOfWorkFactory

type UnitOfWork interface {
	// Transaction controls
	Begin(ctx context.Context) error
	Commit() error
	Rollback() error

	// Repository access (embed to expose methods)
	VenueRepository
	ValidationRepository
}

// UnitOfWorkFactory starts new UnitOfWork instances.
// A returned UnitOfWork is already begun; Begin may be a no-op.
// Keeping Begin on UnitOfWork allows reusing implementations in tests.
type UnitOfWorkFactory interface {
	Begin(ctx context.Context) (UnitOfWork, error)
}
