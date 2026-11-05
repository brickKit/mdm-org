//go:build integration

package org

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evidenceOf returns the records the test deployment's evidence store keeps about one object, by phase.
func evidenceOf(w *sectest.World, object string) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, raw := range w.Evidence() {
		var r map[string]any
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		if o, _ := r["object"].(map[string]any); o != nil && o["id"] == object {
			out[r["phase"].(string)] = append(out[r["phase"].(string)], r)
		}
	}
	return out
}

// evidenceStory is item 4's T6c (assembly protocol 15's evidence table, be-protocol P12.15-P12.18): a tightening of
// the organization (a unit retired, a unit moved out from under a unit above it, a legal entity suspended or retired)
// writes its intent before the change and its conclusion in the change's transaction; one that does not take place is
// cancelled; new units, renames, a move that only adds units above and a legal entity made active again write none;
// with the evidence store down the change goes on and its conclusion waits locally.
func evidenceStory(t *testing.T, w *sectest.World, admin *pgx.Conn, alice client, le string) {
	ctx := context.Background()
	unit := func(code string, parent any) string {
		c, u := alice.do("POST", "/api/org/units", map[string]any{"legal_entity_id": le, "parent_id": parent, "code": code, "name": code})
		require.Equal(t, 200, c, u)
		return u["id"].(string)
	}
	await := func(id, phase string, n int) map[string][]map[string]any {
		t.Helper()
		var got map[string][]map[string]any
		require.Eventually(t, func() bool { got = evidenceOf(w, id); return len(got[phase]) >= n }, 10*time.Second,
			100*time.Millisecond, "%s: %d %s record(s)", id, n, phase)
		return got
	}
	e1, e3 := unit("EV1", nil), unit("EV3", nil)
	e2 := unit("EV2", e1)
	assert.Empty(t, evidenceOf(w, e2), "a new unit is no tightening")

	// moved out from under EV1: the intent, then the effective conclusion with its position and history row
	code, out := alice.do("POST", "/api/org/units/"+e2+"/move", map[string]any{"parent_id": e3})
	require.Equal(t, 200, code, out)
	got := await(e2, "effective", 1)
	require.Len(t, got["intent"], 1)
	in, eff := got["intent"][0], got["effective"][0]
	assert.Equal(t, in["id"], eff["id"], "one record, two phases")
	assert.Equal(t, "org_tightening", in["category"])
	assert.Equal(t, "move_out", in["change"].(map[string]any)["kind"])
	assert.Equal(t, map[string]any{"type": UnitType, "id": e2, "version": "1"}, in["object"], "the version before the move")
	assert.Equal(t, e3, in["change"].(map[string]any)["parent_after"])
	assert.Nil(t, in["position"])
	assert.NotNil(t, eff["position"])
	var hist int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_change_history WHERE evidence_id = $1",
		eff["id"]).Scan(&hist))
	assert.Equal(t, 1, hist, "reflected by its id")

	// a rename and a move that only adds a unit above (EV3 > EV4 > EV2): nothing new is concluded
	e4 := unit("EV4", e3)
	code, out = alice.do("PATCH", "/api/org/units/"+e2, map[string]any{"name": "EV2 renamed"})
	require.Equal(t, 200, code, out)
	code, out = alice.do("POST", "/api/org/units/"+e2+"/move", map[string]any{"parent_id": e4})
	require.Equal(t, 200, code, out)
	got = await(e2, "cancelled", 1)
	assert.Len(t, got["effective"], 1, "a widening move concludes no tightening")
	assert.Len(t, got["intent"], 2, "every move writes its intent: whether it tightens is known under the tree's lock")

	// retired: intent and effective; refused (an active unit below): the intent is cancelled; already retired: nothing
	code, out = alice.do("POST", "/api/org/units/"+e1+"/retire", nil)
	require.Equal(t, 200, code, out)
	got = await(e1, "effective", 1)
	assert.Equal(t, "retirement", got["effective"][0]["change"].(map[string]any)["kind"])
	code, _ = alice.do("POST", "/api/org/units/"+e3+"/retire", nil)
	require.Equal(t, 409, code)
	got = await(e3, "cancelled", 1)
	assert.Len(t, got["intent"], 1)
	assert.Empty(t, got["effective"])
	assert.Nil(t, got["cancelled"][0]["position"])
	code, _ = alice.do("POST", "/api/org/units/"+e1+"/retire", nil)
	require.Equal(t, 200, code)
	assert.Len(t, evidenceOf(w, e1)["intent"], 1, "retiring a retired unit is no change")

	// a legal entity suspended: two phases; made active again: no record
	code, ev := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "EV01", "name": "Evidence Ltd",
		"country": "GB", "timezone": "Europe/London", "base_currency": "GBP"})
	require.Equal(t, 200, code, ev)
	evID := ev["id"].(string)
	code, out = alice.do("POST", "/api/org/legal-entities/"+evID+"/state", map[string]any{"state": "suspended"})
	require.Equal(t, 200, code, out)
	got = await(evID, "effective", 1)
	assert.Equal(t, "legal_entity_status", got["intent"][0]["change"].(map[string]any)["kind"])
	assert.Equal(t, LegalEntityType, got["intent"][0]["object"].(map[string]any)["type"])
	assert.Equal(t, "suspended", got["intent"][0]["change"].(map[string]any)["state"])
	code, out = alice.do("POST", "/api/org/legal-entities/"+evID+"/state", map[string]any{"state": "active"})
	require.Equal(t, 200, code, out)
	assert.Len(t, evidenceOf(w, evID)["intent"], 1, "a widening is not recorded")

	// the evidence store down: a retirement goes on; its conclusion waits in the evidence outbox until it is back
	w.EvidenceDown(true)
	code, out = alice.do("POST", "/api/org/units/"+e2+"/retire", nil)
	require.Equal(t, 200, code, out)
	assert.Equal(t, "retired", out["state"], "the tightening is not held back")
	var waiting int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_evidence_outbox WHERE phase = 'effective'").Scan(&waiting))
	assert.Positive(t, waiting, "the conclusion waits locally")
	w.EvidenceDown(false)
	got = await(e2, "effective", 2)
	assert.Len(t, got["intent"], 2, "no store, no alert channel: the third intent was a double failure")
}
