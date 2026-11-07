//go:build integration

package itest

import (
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

// timezones: no time zone in the request and none configured is refused (protocol 14, no implicit default); with
// the deployment's default it is written into the record; the same creation again returns it; a code is taken once.
func (fx *fixture) timezones() {
	t := fx.t
	// no time zone in the request and none configured: refused (protocol 14, no implicit default)
	stop := run(t, fx.env, fx.logs)
	code, out := fx.alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 400, code, out)
	stop()

	// with the deployment's default, written into the record
	fx.env["DEFAULT_LEGAL_ENTITY_TIMEZONE"] = "Asia/Shanghai"
	stop = run(t, fx.env, fx.logs)
	t.Cleanup(stop)
	code, cn := fx.alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	require.Equal(t, 200, code, cn)
	assert.Equal(t, "Asia/Shanghai", cn["timezone"])
	code, again := fx.alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 200, code)
	assert.Equal(t, cn["id"], again["id"], "the same creation again returns it")
	code, taken := fx.alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "另一个",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 409, code)
	assert.Equal(t, "CODE_TAKEN", taken["reason"])
	fx.cn = cn
}

// businessDate: the boundary card's story: a legal entity in Spain dates its business in Europe/Madrid; Bob sees
// nothing.
func (fx *fixture) businessDate() {
	t := fx.t
	// the boundary card's story: a legal entity in Spain dates its business in Europe/Madrid
	_, es := fx.alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "ES01", "name": "Demo Iberia S.L.",
		"country": "ES", "timezone": "Europe/Madrid", "base_currency": "EUR"})
	fx.esID = es["id"].(string)
	_, d := fx.alice.do("GET", "/api/org/legal-entities/"+fx.esID+"/business-date?at=2026-10-19T22:30:00Z", nil)
	assert.Equal(t, "2026-10-20", d["date"], "00:30 in Madrid (CEST) is already the next day")
	_, d = fx.alice.do("GET", "/api/org/legal-entities/"+fx.cn["id"].(string)+"/business-date?at=2026-10-19T15:30:00Z", nil)
	assert.Equal(t, "2026-10-19", d["date"])
	code, _ := fx.bob.do("GET", "/api/org/legal-entities/"+fx.esID, nil)
	assert.Equal(t, 404, code, "Bob holds no grant: not visible")
}

// notInThisVersion: positions and the rest answer CAPABILITY_UNSUPPORTED (D75).
func (fx *fixture) notInThisVersion() {
	t := fx.t
	// not in step 6
	code, unsup := fx.alice.do("GET", "/api/org/positions", nil)
	assert.Equal(t, "CAPABILITY_UNSUPPORTED", unsup["reason"], code)
}
