package org

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/brickKit/be-sdk-go/v3/httpx"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/mergesafety"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheManifestDeclaresWhatTheSDKReads: component.yaml's configSchema is exactly the keys the SDK reads.
func TestTheManifestDeclaresWhatTheSDKReads(t *testing.T) {
	raw, err := os.ReadFile("../component.yaml")
	require.NoError(t, err)
	if p := launcher.ManifestProblems(raw, Definition(), Own); len(p) > 0 {
		t.Errorf("component.yaml's configSchema differs from what the SDK reads:\n%v\nthe block:\n%s", p,
			launcher.ConfigSchemaYAML(Definition(), Own))
	}
}

// TestTheManifestListsThePublishedEvents: component.yaml's events.publishes is exactly the event contracts.
func TestTheManifestListsThePublishedEvents(t *testing.T) {
	raw, err := os.ReadFile("../component.yaml")
	require.NoError(t, err)
	assert.Empty(t, launcher.EventProblems(raw, os.DirFS("../contract/events")))
}

// TestTheRoutesAreTheOpenAPIDocument: every route is in contract/openapi.yaml with its operation, and nothing else is.
func TestTheRoutesAreTheOpenAPIDocument(t *testing.T) {
	raw, err := os.ReadFile("../contract/openapi.yaml")
	require.NoError(t, err)
	var r httpx.Routes
	require.NoError(t, (&Module{}).Routes(&r))
	assert.Empty(t, httpx.OpenAPIProblems(raw, r.List()))
}

// TestThePayloadsFitTheirSchemas: what events and source reads carry is what contract/events promises.
func TestThePayloadsFitTheirSchemas(t *testing.T) {
	parent := "01a15ab0-0000-7000-8000-0000000000b1"
	for typ, v := range map[string]any{
		LegalEntityChanged: LegalEntity{ID: "01a15ab0-0000-7000-8000-0000000000a1", Code: "ES01", Name: "Demo Iberia S.L.",
			Names: map[string]string{"zh-CN": "演示伊比利亚"}, Country: "ES", Timezone: "Europe/Madrid", BaseCurrency: "EUR",
			FiscalYearStartMonth: 1, State: "active", Version: 1, UpdatedAt: "2026-10-20T08:00:00.000000Z"},
		UnitChanged: Unit{ID: "01a15ab0-0000-7000-8000-0000000000b2", LegalEntityID: "01a15ab0-0000-7000-8000-0000000000a1",
			ParentID: &parent, Code: "SALES-N", Name: "Sales North", Names: map[string]string{},
			Path:  "01a15ab0-0000-7000-8000-0000000000a1/01a15ab0-0000-7000-8000-0000000000b1/01a15ab0-0000-7000-8000-0000000000b2",
			State: "active", Version: 2, UpdatedAt: "2026-10-20T08:00:00.000000Z"},
	} {
		c := jsonschema.NewCompiler()
		s, err := c.Compile("../contract/events/" + typ + ".schema.json")
		require.NoError(t, err, typ)
		b, err := json.Marshal(v)
		require.NoError(t, err)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		require.NoError(t, err)
		assert.NoError(t, s.Validate(doc), typ)
	}
}

// TestTheModuleIsMergeSafe: the module is compiled into the Go infra shell.
func TestTheModuleIsMergeSafe(t *testing.T) {
	findings, err := mergesafety.CheckDir(".")
	require.NoError(t, err)
	for _, f := range findings {
		t.Errorf("%v", f)
	}
}
