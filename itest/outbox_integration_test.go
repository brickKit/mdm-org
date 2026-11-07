//go:build integration

package itest

import (
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// outbox: the move published the moved unit and every unit below it.
func (fx *fixture) outbox() {
	t, ctx := fx.t, fx.ctx
	// the move published the moved unit and every unit below it
	var n int
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_outbox WHERE record_id = $1",
		fx.city["id"]).Scan(&n))
	assert.Equal(t, 2, n, "created, then moved with its parent")
}
