//go:build integration

package itest

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tightening "github.com/brickKit/mdm-org/v3/internal/tightening"

	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	core "github.com/brickKit/mdm-org/v3/internal/core"
)

// positions (3.0.34): a position belongs to one active unit of an active legal entity; its code is unique within the
// legal entity (the same creation again answers the same position); renaming makes a new version; a unit with an
// active position cannot retire; retiring a position is a tightening (its intent, then its conclusion) and a full
// recovery replays it; the event and the source read carry the position; without a grant it is not there.
func (fx *fixture) positions() {
	t, ctx := fx.t, fx.ctx
	unit := func(code string) map[string]any {
		c, u := fx.alice.do("POST", "/api/org/units", map[string]any{"legal_entity_id": fx.esID, "parent_id": nil,
			"code": code, "name": code})
		require.Equal(t, 200, c, u)
		return u
	}
	pu := unit("POSU")
	create := func(unitID, code, name string) (int, map[string]any) {
		return fx.alice.do("POST", "/api/org/positions", map[string]any{"unit_id": unitID, "code": code, "name": name})
	}

	// created in an active unit; the legal entity comes from the unit
	code, mgr := create(pu["id"].(string), "MGR", "Manager")
	require.Equal(t, 200, code, mgr)
	assert.Equal(t, fx.esID, mgr["legal_entity_id"])
	assert.Equal(t, pu["id"], mgr["unit_id"])
	assert.Equal(t, "active", mgr["state"])
	assert.EqualValues(t, 1, mgr["version"])
	id := mgr["id"].(string)

	// the same creation again answers the same position; the code is the legal entity's once
	code, again := create(pu["id"].(string), "MGR", "Manager")
	require.Equal(t, 200, code, again)
	assert.Equal(t, id, again["id"])
	code, taken := create(pu["id"].(string), "MGR", "Another")
	assert.Equal(t, 409, code)
	assert.Equal(t, "CODE_TAKEN", taken["reason"])
	other := unit("POSV")
	code, taken = create(other["id"].(string), "MGR", "Manager")
	assert.Equal(t, 409, code, "unique within the legal entity, not the unit")
	assert.Equal(t, "CODE_TAKEN", taken["reason"])

	// not in a retired unit (treeRules retired SALES), not in a unit that does not exist
	code, refused := create(fx.sales["id"].(string), "CLERK", "Clerk")
	assert.Equal(t, 409, code)
	assert.Equal(t, "STATE_CONFLICT", refused["reason"])
	code, _ = create("01a1c4fe-adb1-7cec-af26-93a07fc8b644", "CLERK", "Clerk")
	assert.Equal(t, 400, code)

	// renamed: a new version; the code stays
	code, renamed := fx.alice.do("PATCH", "/api/org/positions/"+id, map[string]any{"version": 1, "name": "Sales Manager",
		"names": map[string]string{"es": "Jefe de ventas"}})
	require.Equal(t, 200, code, renamed)
	assert.Equal(t, "Sales Manager", renamed["name"])
	assert.EqualValues(t, 2, renamed["version"])
	code, stale := fx.alice.do("PATCH", "/api/org/positions/"+id, map[string]any{"version": 1, "name": "x"})
	assert.Equal(t, 409, code)
	assert.Equal(t, "VERSION_CONFLICT", stale["reason"])

	// listed by unit and by legal entity
	code, clerk := create(pu["id"].(string), "CLERK", "Clerk")
	require.Equal(t, 200, code, clerk)
	_, byUnit := fx.alice.do("GET", "/api/org/positions?unit_id="+pu["id"].(string), nil)
	codes := []string{}
	for _, p := range byUnit["positions"].([]any) {
		codes = append(codes, p.(map[string]any)["code"].(string))
	}
	assert.Equal(t, []string{"CLERK", "MGR"}, codes, "by code")
	_, byEntity := fx.alice.do("GET", "/api/org/positions?legal_entity_id="+fx.esID+"&limit=1", nil)
	assert.Len(t, byEntity["positions"], 1)
	assert.NotEmpty(t, byEntity["next_cursor"])

	// without a grant: not there
	code, _ = fx.bob.do("GET", "/api/org/positions/"+id, nil)
	assert.Equal(t, 404, code)

	// a unit with an active position does not retire
	code, busy := fx.alice.do("POST", "/api/org/units/"+pu["id"].(string)+"/retire", nil)
	assert.Equal(t, 409, code)
	assert.Equal(t, "STATE_CONFLICT", busy["reason"])

	// retiring a position: its intent first, then its conclusion (a tightening, protocol 15); again: no change
	code, retired := fx.alice.do("POST", "/api/org/positions/"+id+"/retire", nil)
	require.Equal(t, 200, code, retired)
	assert.Equal(t, "retired", retired["state"])
	code, twice := fx.alice.do("POST", "/api/org/positions/"+id+"/retire", nil)
	require.Equal(t, 200, code)
	assert.Equal(t, retired["version"], twice["version"], "retiring a retired position changes nothing")
	var ev map[string][]map[string]any
	require.Eventually(t, func() bool { ev = evidenceOf(fx.w, id); return len(ev["effective"]) == 1 }, 10*time.Second,
		100*time.Millisecond)
	require.Len(t, ev["intent"], 1)
	obj := ev["intent"][0]["object"].(map[string]any)
	assert.Equal(t, tightening.PositionType, obj["type"])
	assert.Equal(t, "2", obj["version"], "the version before the change")
	assert.Equal(t, "retirement", ev["intent"][0]["change"].(map[string]any)["kind"])
	code, _ = create(pu["id"].(string), "MGR2", "Manager")
	require.Equal(t, 200, code)
	_, _ = fx.alice.do("PATCH", "/api/org/positions/"+clerk["id"].(string), map[string]any{"name": "Senior Clerk"})
	assert.Empty(t, evidenceOf(fx.w, clerk["id"].(string)), "a new position and a rename tighten nothing")

	// no position can be created in a retired position's place under the old code
	code, taken = create(pu["id"].(string), "MGR", "Manager")
	assert.Equal(t, 409, code, "a retired position keeps its code")

	// the event: the full state, in the change's transaction
	var events int
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_outbox WHERE type = $1 AND record_id = $2",
		core.PositionChanged, id).Scan(&events))
	assert.Equal(t, 3, events, "created, renamed, retired")

	// the source read (system surface)
	tls := fx.w.TLS(t, "mdm/partner")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(core.ID))))
	require.NoError(t, err)
	defer conn.Close()
	read, err := orgv3.NewSourceServiceClient(conn).Read(ctx, &orgv3.ReadRequest{Aggregate: orgv3.Aggregate_AGGREGATE_POSITION,
		Ids: []string{id}})
	require.NoError(t, err)
	require.Len(t, read.Results, 1)
	assert.Equal(t, orgv3.ReadState_READ_STATE_EXISTS, read.Results[0].State)
	assert.Contains(t, string(read.Results[0].Data), `"state":"retired"`)

	// a full recovery replays the retirement as recorded
	positionReplay(fx, id, ev["effective"][0])
}

// positionReplay puts the position back as it was before the retirement, then replays the retirement's conclusion.
func positionReplay(fx *fixture, id string, rec map[string]any) {
	t, ctx := fx.t, context.Background()
	var raw []byte
	for k, v := range fx.w.Evidence() {
		if strings.HasSuffix(k, "/effective") {
			var r map[string]any
			if json.Unmarshal(v, &r) == nil && r["id"] == rec["id"] {
				raw = v
			}
		}
	}
	require.NotNil(t, raw)
	_, err := fx.admin.Exec(ctx, "UPDATE "+itSchema+".org_position SET state = 'active' WHERE id = $1", id)
	require.NoError(t, err)
	_, err = fx.admin.Exec(ctx, "DELETE FROM "+itSchema+".be_change_history WHERE evidence_id = $1", rec["id"])
	require.NoError(t, err)
	tls := fx.w.TLS(t, "ops/recover")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(core.ID))))
	require.NoError(t, err)
	defer conn.Close()
	outcome, _, err := evidence.ReplayClient{Conn: conn}.Replay(ctx, raw, "r1")
	require.NoError(t, err)
	assert.Equal(t, evidence.Applied, outcome)
	var state string
	require.NoError(t, fx.admin.QueryRow(ctx, "SELECT state FROM "+itSchema+".org_position WHERE id = $1", id).Scan(&state))
	assert.Equal(t, "retired", state)
}
