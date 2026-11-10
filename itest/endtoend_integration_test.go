//go:build integration

package itest

// The organization component against PostgreSQL and the bus over its real surfaces (L2), with tokens and service
// certificates of be-sdk-go's sectest deployment. Needs BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL, as be-sdk-go's
// integration tests; missing, it fails. The member listens on 28083 (user) and 9093 (system).
// Run: go test -tags integration ./itest/ (make test-integration); the index of the steps is itest/AGENTS.md

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	org "github.com/brickKit/mdm-org/v3/org"

	core "github.com/brickKit/mdm-org/v3/internal/core"

	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const (
	itPort   = "28083"
	itBase   = "http://localhost:" + itPort
	itSystem = "localhost:9093"
	itSchema = "mdm_org_it"
)

type client struct {
	t     *testing.T
	token string
}

func (c client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, itBase+path, r)
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// run runs the component until the returned stop is called.
func run(t *testing.T, env map[string]string, logs *bytes.Buffer) func() {
	opts := launcher.Options{Stdout: logs, Stderr: logs,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	sctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, org.Definition(), opts) }()
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 20*time.Second, 100*time.Millisecond, logs.String())
	return func() { stop(); <-done }
}

// fixture is the migrated schema, the world and what one step leaves for the next; the member runs from the first
// step on (it starts it twice, with and without a default time zone).
type fixture struct {
	t                       *testing.T
	ctx                     context.Context
	admin                   *pgx.Conn
	w                       *sectest.World
	env                     map[string]string
	logs                    *bytes.Buffer
	alice, bob              client
	cn                      map[string]any // the Chinese legal entity (timezones)
	esID                    string         // the Spanish one (businessDate)
	sales, north, city, ops map[string]any // its units (tree)
}

func TestOrgEndToEnd(t *testing.T) {
	fx := start(t)
	fx.timezones()
	fx.businessDate()
	fx.tree()
	fx.treeRules()
	fx.lists()
	fx.outbox()
	fx.positions()
	fx.notInThisVersion()
	fx.source()
	fx.evidence()
	fx.replay()
	fx.recoveryChecks()
}

// start makes the world and migrates; the member is not running yet.
func start(t *testing.T) *fixture {
	adminURL, natsURL := os.Getenv("BE_SDK_TEST_PG_ADMIN"), os.Getenv("BE_SDK_TEST_NATS_URL")
	if adminURL == "" || natsURL == "" {
		t.Fatal("BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL are needed")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close(ctx) })
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)
	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, core.Definitions())
	portFree(t, itPort, itSystem)
	env := w.Env(t, core.ID)
	for k, v := range map[string]string{"COMPONENT_ID": core.ID, "COMPONENT_VERSION": org.Version, "PORT": itPort,
		"STOP_DRAIN_DELAY": "50ms", "PG_HOST": u.Hostname(), "PG_PORT": u.Port(), "PG_DATABASE": strings.TrimPrefix(u.Path, "/"),
		"PG_USER": u.User.Username(), "PG_PASSWORD": pw, "PG_SCHEMA": itSchema, "PG_OWNER_USER": u.User.Username(),
		"PG_OWNER_PASSWORD": pw, "BUS_URL": natsURL, "IMPLEMENTATION_INSTANCE": ids.New().String()} {
		env[k] = v
	}
	var logs bytes.Buffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(logs.String())
		}
	})
	migrate := launcher.Options{Args: []string{"migrate"}, Stdout: &logs, Stderr: &logs,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, org.Definition(), migrate))
	return &fixture{t: t, ctx: ctx, admin: admin, w: w, env: env, logs: &logs, alice: client{t, w.Bearer(sectest.Alice)},
		bob: client{t, w.Bearer(sectest.Bob)}}
}

// portFree fails at once when a port the member needs is taken: a running cell (host ports 18xxx) or another member
// would answer in its place, and the test would assert against another process.
func portFree(t *testing.T, addrs ...string) {
	for _, a := range addrs {
		if !strings.Contains(a, ":") {
			a = "localhost:" + a
		}
		ln, err := net.Listen("tcp", a)
		require.NoError(t, err, "port %s is taken: is a cell or another member running?", a)
		_ = ln.Close()
	}
}
