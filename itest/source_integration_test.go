//go:build integration

package itest

import (
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/ids"
	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	core "github.com/brickKit/mdm-org/v3/internal/core"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// source: the system surface over mTLS: Read (exists, purged, not an ID), the stamp, Snapshot pages, CatchUp not yet.
func (fx *fixture) source() {
	t, ctx := fx.t, fx.ctx
	// the system surface over mTLS
	tls := fx.w.TLS(t, "mdm/partner")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(core.ID))))
	require.NoError(t, err)
	defer conn.Close()
	read, err := orgv3.NewSourceServiceClient(conn).Read(ctx, &orgv3.ReadRequest{Aggregate: orgv3.Aggregate_AGGREGATE_LEGAL_ENTITY,
		Ids: []string{fx.esID, ids.New().String(), "not-an-id"}})
	require.NoError(t, err)
	require.Len(t, read.Results, 3)
	assert.Equal(t, orgv3.ReadState_READ_STATE_EXISTS, read.Results[0].State)
	assert.Contains(t, string(read.Results[0].Data), `"timezone":"Europe/Madrid"`)
	assert.Equal(t, orgv3.ReadState_READ_STATE_PURGED, read.Results[1].State)
	assert.Equal(t, orgv3.ReadState_READ_STATE_PURGED, read.Results[2].State)
	assert.Len(t, read.Stamp.Positions, 4)
	snap, err := orgv3.NewSourceServiceClient(conn).Snapshot(ctx, &orgv3.SnapshotRequest{Aggregate: orgv3.Aggregate_AGGREGATE_ORG_UNIT, Limit: 3})
	require.NoError(t, err)
	assert.Len(t, snap.Records, 3)
	assert.NotEmpty(t, snap.NextCursor)
	_, err = orgv3.NewCatchUpServiceClient(conn).CatchUp(ctx, &orgv3.CatchUpRequest{Nonce: "x"})
	e, _, ok := errs.FromGRPC(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, errs.ReasonCapabilityUnsupported, e.Reason)
}
