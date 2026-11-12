// Package org is mdm/org (boundary card mdm/org): legal entities (with their time zone, base currency and the fiscal
// year's start month), the organization units below them, whose paths are the org-tree facts of authorization
// (security 07), and the positions in the units (3.0.34; mdm/employee's appointments hang on them). Kept by people
// over the user surface, read by components over the system surface (protocol 02's source methods), published as
// full-state events. Reporting lines, locations, work calendars, effective-dated changes and the watermark catch-up
// answer CAPABILITY_UNSUPPORTED (D75).
package org

import (
	"embed"
	"io/fs"

	core "github.com/brickKit/mdm-org/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/component"
	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/events"
	"github.com/brickKit/be-sdk-go/v3/runtime"
)

// Identity of the component.
const (
	Version  = "3.0.38"
	GRPCPort = 9093
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func migrations() fs.FS {
	sub, _ := fs.Sub(migrationFiles, "migrations")
	return sub
}

// Keys are the component's own configuration keys.
func Keys() []config.Key {
	return []config.Key{
		// the time zone a legal entity gets when its creation gives none (protocol 14); written into the record, so a
		// later change of the key changes no existing legal entity. Unset: a creation without a time zone is refused
		{Name: "DEFAULT_LEGAL_ENTITY_TIMEZONE", Type: config.String},
	}
}

// Own describes the own keys for the manifest's configSchema.
var Own = map[string]string{"DEFAULT_LEGAL_ENTITY_TIMEZONE": "the IANA time zone a legal entity gets when its " +
	"creation gives none, written into the record; unset: a creation without a time zone is refused"}

// Definition is the component as the launcher and the shells take it.
func Definition() component.Definition {
	return component.Definition{ID: core.ID, Version: Version, Keys: Keys(), Migrations: migrations(), GRPCPort: GRPCPort,
		Streams: []events.Stream{{Name: core.Stream, Partitions: 4}}, Authz: core.Definitions(), Resend: core.OpResend, Recovery: core.OpRecover, Replay: core.OpReplay, Evidence: true,
		New: func(rt *runtime.Runtime) (component.Module, error) { return New(rt) }}
}
