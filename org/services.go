package org

import (
	"context"

	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	source "github.com/brickKit/mdm-org/v3/internal/source"
)

// The gRPC methods of the system surface answer through internal/source.

// Snapshot is SourceService.Snapshot.
func (m *Module) Snapshot(ctx context.Context, in *orgv3.SnapshotRequest) (*orgv3.SnapshotResponse, error) {
	return source.Snapshot(m.b, ctx, in)
}

// Read is SourceService.Read.
func (m *Module) Read(ctx context.Context, in *orgv3.ReadRequest) (*orgv3.ReadResponse, error) {
	return source.Read(m.b, ctx, in)
}

// CatchUp is CatchUpService.CatchUp.
func (m *Module) CatchUp(ctx context.Context, in *orgv3.CatchUpRequest) (*orgv3.CatchUpResponse, error) {
	return source.CatchUp(m.b, ctx, in)
}
