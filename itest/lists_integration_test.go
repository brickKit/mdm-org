//go:build integration

package itest

import (
	"net/url"

	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// lists: units by legal entity, in path order, by cursor; the legal entities' keyword (pinyin initials).
func (fx *fixture) lists() {
	t := fx.t
	// lists: by legal entity, in path order, by cursor; the keyword
	_, page1 := fx.alice.do("GET", "/api/org/units?legal_entity_id="+fx.esID+"&limit=2", nil)
	require.Len(t, page1["units"], 2)
	_, page2 := fx.alice.do("GET", "/api/org/units?legal_entity_id="+fx.esID+"&limit=2&cursor="+url.QueryEscape(page1["next_cursor"].(string)), nil)
	require.Len(t, page2["units"], 2)
	assert.Empty(t, page2["next_cursor"])
	_, found := fx.alice.do("GET", "/api/org/legal-entities?q=yszg", nil) // 演示中国: y s z g
	require.Len(t, found["legal_entities"], 1)
}
