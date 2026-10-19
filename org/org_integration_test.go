//go:build integration

package org

// The organization component against PostgreSQL and the bus over its real surfaces (L2), with tokens and service
// certificates of be-sdk-go's sectest deployment. Needs BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL, as be-sdk-go's
// integration tests; missing, it fails. The member listens on 18083 (user) and 9093 (system).
// Run: go test -tags integration ./org/

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brickKit/be-sdk-go/v3/config"
	"github.com/brickKit/be-sdk-go/v3/errs"
	"github.com/brickKit/be-sdk-go/v3/ids"
	"github.com/brickKit/be-sdk-go/v3/launcher"
	"github.com/brickKit/be-sdk-go/v3/sectest"
	orgv3 "github.com/brickKit/mdm-org/contract/v3/gen/go/be/mdm/org/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	itPort   = "18083"
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

// start runs the component until the returned stop is called.
func start(t *testing.T, env map[string]string, logs *bytes.Buffer) func() {
	opts := launcher.Options{Stdout: logs, Stderr: logs,
		Env: config.Env{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }, Read: os.ReadFile}}
	sctx, stop := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- launcher.Run(sctx, Definition(), opts) }()
	require.Eventually(t, func() bool {
		r, err := http.Get(itBase + "/readyz")
		if err == nil {
			r.Body.Close()
		}
		return err == nil && r.StatusCode == 200
	}, 20*time.Second, 100*time.Millisecond, logs.String())
	return func() { stop(); <-done }
}

func TestOrgEndToEnd(t *testing.T) {
	adminURL, natsURL := os.Getenv("BE_SDK_TEST_PG_ADMIN"), os.Getenv("BE_SDK_TEST_NATS_URL")
	if adminURL == "" || natsURL == "" {
		t.Fatal("BE_SDK_TEST_PG_ADMIN and BE_SDK_TEST_NATS_URL are needed")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+itSchema+" CASCADE; CREATE SCHEMA "+itSchema)
	require.NoError(t, err)
	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()

	w := sectest.New(t)
	w.GrantAll(sectest.Alice, Definitions())
	env := w.Env(t, ID)
	for k, v := range map[string]string{"COMPONENT_ID": ID, "COMPONENT_VERSION": Version, "PORT": itPort,
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
	require.Equal(t, launcher.ExitOK, launcher.Run(ctx, Definition(), migrate))
	alice, bob := client{t, w.Bearer(sectest.Alice)}, client{t, w.Bearer(sectest.Bob)}

	// no time zone in the request and none configured: refused (protocol 14, no implicit default)
	stop := start(t, env, &logs)
	code, out := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 400, code, out)
	stop()

	// with the deployment's default, written into the record
	env["DEFAULT_LEGAL_ENTITY_TIMEZONE"] = "Asia/Shanghai"
	stop = start(t, env, &logs)
	defer stop()
	code, cn := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	require.Equal(t, 200, code, cn)
	assert.Equal(t, "Asia/Shanghai", cn["timezone"])
	code, again := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "演示中国",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 200, code)
	assert.Equal(t, cn["id"], again["id"], "the same creation again returns it")
	code, taken := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "CN01", "name": "另一个",
		"country": "CN", "base_currency": "CNY"})
	assert.Equal(t, 409, code)
	assert.Equal(t, "CODE_TAKEN", taken["reason"])

	// the boundary card's story: a legal entity in Spain dates its business in Europe/Madrid
	_, es := alice.do("POST", "/api/org/legal-entities", map[string]any{"code": "ES01", "name": "Demo Iberia S.L.",
		"country": "ES", "timezone": "Europe/Madrid", "base_currency": "EUR"})
	esID := es["id"].(string)
	_, d := alice.do("GET", "/api/org/legal-entities/"+esID+"/business-date?at=2026-10-19T22:30:00Z", nil)
	assert.Equal(t, "2026-10-20", d["date"], "00:30 in Madrid (CEST) is already the next day")
	_, d = alice.do("GET", "/api/org/legal-entities/"+cn["id"].(string)+"/business-date?at=2026-10-19T15:30:00Z", nil)
	assert.Equal(t, "2026-10-19", d["date"])
	code, _ = bob.do("GET", "/api/org/legal-entities/"+esID, nil)
	assert.Equal(t, 404, code, "Bob holds no grant: not visible")

	// a tree: SALES > NORTH > CITY, and OPS; paths are the legal entity, then the units from the top
	unit := func(code string, parent any) map[string]any {
		c, u := alice.do("POST", "/api/org/units", map[string]any{"legal_entity_id": esID, "parent_id": parent,
			"code": code, "name": code})
		require.Equal(t, 200, c, u)
		return u
	}
	sales := unit("SALES", nil)
	north := unit("NORTH", sales["id"])
	city := unit("CITY", north["id"])
	ops := unit("OPS", nil)
	assert.Equal(t, esID+"/"+sales["id"].(string)+"/"+north["id"].(string)+"/"+city["id"].(string), city["path"])

	// the story: a department moves; the paths below it follow in the same change, the old ones stay traceable
	code, moved := alice.do("POST", "/api/org/units/"+north["id"].(string)+"/move", map[string]any{"parent_id": ops["id"]})
	require.Equal(t, 200, code, moved)
	assert.Equal(t, esID+"/"+ops["id"].(string)+"/"+north["id"].(string), moved["path"])
	_, city2 := alice.do("GET", "/api/org/units/"+city["id"].(string), nil)
	assert.Equal(t, moved["path"].(string)+"/"+city["id"].(string), city2["path"])
	assert.EqualValues(t, 2, city2["version"], "a new path is a new version")
	_, hist := alice.do("GET", "/api/org/units/"+city["id"].(string)+"/path-history", nil)
	h := hist["history"].([]any)
	require.Len(t, h, 2)
	assert.Equal(t, city["path"], h[0].(map[string]any)["path"])
	assert.NotNil(t, h[0].(map[string]any)["valid_to"])
	assert.Nil(t, h[1].(map[string]any)["valid_to"])
	_, anc := alice.do("GET", "/api/org/units/"+city["id"].(string)+"/ancestors", nil)
	a := anc["ancestors"].([]any)
	require.Len(t, a, 2)
	assert.Equal(t, "OPS", a[0].(map[string]any)["code"])
	assert.Equal(t, "NORTH", a[1].(map[string]any)["code"])

	// no cycles; no retiring above active units
	code, cyc := alice.do("POST", "/api/org/units/"+ops["id"].(string)+"/move", map[string]any{"parent_id": city["id"]})
	assert.Equal(t, 409, code)
	assert.Equal(t, "TREE_CYCLE", cyc["reason"])
	code, busy := alice.do("POST", "/api/org/units/"+ops["id"].(string)+"/retire", nil)
	assert.Equal(t, 409, code)
	assert.Equal(t, "STATE_CONFLICT", busy["reason"])
	code, retired := alice.do("POST", "/api/org/units/"+sales["id"].(string)+"/retire", nil)
	assert.Equal(t, 200, code, retired)
	assert.Equal(t, "retired", retired["state"])

	// lists: by legal entity, in path order, by cursor; the keyword
	_, page1 := alice.do("GET", "/api/org/units?legal_entity_id="+esID+"&limit=2", nil)
	require.Len(t, page1["units"], 2)
	_, page2 := alice.do("GET", "/api/org/units?legal_entity_id="+esID+"&limit=2&cursor="+url.QueryEscape(page1["next_cursor"].(string)), nil)
	require.Len(t, page2["units"], 2)
	assert.Empty(t, page2["next_cursor"])
	_, found := alice.do("GET", "/api/org/legal-entities?q=yszg", nil) // 演示中国: y s z g
	require.Len(t, found["legal_entities"], 1)

	// the move published the moved unit and every unit below it
	var n int
	require.NoError(t, admin.QueryRow(ctx, "SELECT count(*) FROM "+itSchema+".be_outbox WHERE record_id = $1",
		city["id"]).Scan(&n))
	assert.Equal(t, 2, n, "created, then moved with its parent")

	// not in step 6
	code, unsup := alice.do("GET", "/api/org/positions", nil)
	assert.Equal(t, "CAPABILITY_UNSUPPORTED", unsup["reason"], code)

	// the system surface over mTLS
	tls := w.TLS(t, "mdm/partner")
	conn, err := grpc.NewClient(itSystem, grpc.WithTransportCredentials(credentials.NewTLS(tls.ClientConfig(ID))))
	require.NoError(t, err)
	defer conn.Close()
	read, err := orgv3.NewSourceServiceClient(conn).Read(ctx, &orgv3.ReadRequest{Aggregate: orgv3.Aggregate_AGGREGATE_LEGAL_ENTITY,
		Ids: []string{esID, ids.New().String(), "not-an-id"}})
	require.NoError(t, err)
	require.Len(t, read.Results, 3)
	assert.Equal(t, orgv3.ReadState_READ_STATE_EXISTS, read.Results[0].State)
	assert.Contains(t, string(read.Results[0].Data), `"timezone":"Europe/Madrid"`)
	assert.Equal(t, orgv3.ReadState_READ_STATE_PURGED, read.Results[1].State)
	assert.Equal(t, orgv3.ReadState_READ_STATE_PURGED, read.Results[2].State)
	assert.Len(t, read.Stamp.Positions, 4)
	snap, err := orgv3.NewSourceServiceClient(conn).Snapshot(ctx, &orgv3.SnapshotRequest{Aggregate: orgv3.Aggregate_AGGREGATE_ORG_UNIT, Limit: 3})
	require.NoError(t, err)
	assert.Len(t, snap.Records, 3)
	assert.NotEmpty(t, snap.NextCursor)
	_, err = orgv3.NewCatchUpServiceClient(conn).CatchUp(ctx, &orgv3.CatchUpRequest{Nonce: "x"})
	e, _, ok := errs.FromGRPC(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, errs.ReasonCapabilityUnsupported, e.Reason)
}
