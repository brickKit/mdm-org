//go:build integration

package itest

import (
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// tree: SALES > NORTH > CITY, and OPS; paths are the legal entity, then the units from the top; a department
// moves, the paths below it follow in the same change, the old ones stay traceable; the ancestors.
func (fx *fixture) tree() {
	t := fx.t
	// a tree: SALES > NORTH > CITY, and OPS; paths are the legal entity, then the units from the top
	unit := func(code string, parent any) map[string]any {
		c, u := fx.alice.do("POST", "/api/org/units", map[string]any{"legal_entity_id": fx.esID, "parent_id": parent,
			"code": code, "name": code})
		require.Equal(t, 200, c, u)
		return u
	}
	sales := unit("SALES", nil)
	north := unit("NORTH", sales["id"])
	city := unit("CITY", north["id"])
	ops := unit("OPS", nil)
	fx.sales, fx.north, fx.city, fx.ops = sales, north, city, ops
	assert.Equal(t, fx.esID+"/"+sales["id"].(string)+"/"+north["id"].(string)+"/"+city["id"].(string), city["path"])

	// the story: a department moves; the paths below it follow in the same change, the old ones stay traceable
	code, moved := fx.alice.do("POST", "/api/org/units/"+north["id"].(string)+"/move", map[string]any{"parent_id": ops["id"]})
	require.Equal(t, 200, code, moved)
	assert.Equal(t, fx.esID+"/"+ops["id"].(string)+"/"+north["id"].(string), moved["path"])
	_, city2 := fx.alice.do("GET", "/api/org/units/"+city["id"].(string), nil)
	assert.Equal(t, moved["path"].(string)+"/"+city["id"].(string), city2["path"])
	assert.EqualValues(t, 2, city2["version"], "a new path is a new version")
	_, hist := fx.alice.do("GET", "/api/org/units/"+city["id"].(string)+"/path-history", nil)
	h := hist["history"].([]any)
	require.Len(t, h, 2)
	assert.Equal(t, city["path"], h[0].(map[string]any)["path"])
	assert.NotNil(t, h[0].(map[string]any)["valid_to"])
	assert.Nil(t, h[1].(map[string]any)["valid_to"])
	_, anc := fx.alice.do("GET", "/api/org/units/"+city["id"].(string)+"/ancestors", nil)
	a := anc["ancestors"].([]any)
	require.Len(t, a, 2)
	assert.Equal(t, "OPS", a[0].(map[string]any)["code"])
	assert.Equal(t, "NORTH", a[1].(map[string]any)["code"])
}

// treeRules: no cycles; no retiring above active units; a unit without active units below retires.
func (fx *fixture) treeRules() {
	t := fx.t
	// no cycles; no retiring above active units
	code, cyc := fx.alice.do("POST", "/api/org/units/"+fx.ops["id"].(string)+"/move", map[string]any{"parent_id": fx.city["id"]})
	assert.Equal(t, 409, code)
	assert.Equal(t, "TREE_CYCLE", cyc["reason"])
	code, busy := fx.alice.do("POST", "/api/org/units/"+fx.ops["id"].(string)+"/retire", nil)
	assert.Equal(t, 409, code)
	assert.Equal(t, "STATE_CONFLICT", busy["reason"])
	code, retired := fx.alice.do("POST", "/api/org/units/"+fx.sales["id"].(string)+"/retire", nil)
	assert.Equal(t, 200, code, retired)
	assert.Equal(t, "retired", retired["state"])
}
