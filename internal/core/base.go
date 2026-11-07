package core

import (
	"github.com/brickKit/be-sdk-go/v3/evidence"
	"github.com/brickKit/be-sdk-go/v3/runtime"
)

// Base is what every module works with: the component's runtime (its database, events, audit, logger); the evidence
// writer of the changes that tighten access (nil: no evidence, the module run alone in tests); the deployment and
// implementation instance the source methods stamp their answers with (protocol 02); the time zone a new legal
// entity gets when none is given (DEFAULT_LEGAL_ENTITY_TIMEZONE).
type Base struct {
	RT          *runtime.Runtime
	Ev          *evidence.Writer
	Deployment  string
	Instance    string
	DefaultZone string
}
