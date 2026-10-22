package blazingagents_test

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var (
	_ = testkit.RedactionCase("CreateMerchantConnectionParams", func(secret string) any {
		return ba.CreateMerchantConnectionParams{Provider: "polar", Environment: "sandbox", Credential: ba.NewSecret(secret)}
	})
	_ = testkit.RedactionCase("UpdateMerchantConnectionParams", func(secret string) any {
		return &ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret(secret)}
	})
)

// merchantInner is the object under key in the response example of opID.
func merchantInner(t *testing.T, opID string, status int, key string) string {
	t.Helper()
	var env map[string]any
	if err := jsonUnmarshalString(testkit.Example(opID, status), &env); err != nil {
		t.Fatal(err)
	}
	return jsonString(env[key])
}

func merchantMatches(example string) func(*testing.T, any) {
	return func(t *testing.T, got any) {
		testkit.MatchesExample(t, example, got)
		testkit.CoversExample(t, example, got)
	}
}

// merchantTwice ranges seq twice, to show that ranging again starts over.
func merchantTwice[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for range 2 {
		for v, err := range seq {
			if err != nil {
				return out, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func merchantErr[T any](_ T, err error) error { return err }

var merchantTenant = http.Header{"Authorization": {"Bearer " + testkit.APIKey}}

func TestMerchantConnection(t *testing.T) {
	const path = "/v1/merchant-connection"
	connection := merchantInner(t, "getMerchantConnection", 200, "connection")
	exchange := func(opID string, status int, body, resp string) []testkit.Exchange {
		return []testkit.Exchange{{Op: opID, Path: path, Status: status, Body: body, Header: merchantTenant, NoHeader: []string{"X-BA-User-Id"}, RespBody: resp}}
	}
	create := func(params ba.CreateMerchantConnectionParams, body string) testkit.Case {
		return testkit.Case{
			Name:      "createMerchantConnection " + body,
			Exchanges: exchange("createMerchantConnection", 201, body, testkit.Example("createMerchantConnection", 201)),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Create(ctx, params)
			},
			Check: merchantMatches(merchantInner(t, "createMerchantConnection", 201, "connection")),
		}
	}
	update := func(params ba.UpdateMerchantConnectionParams, body string) testkit.Case {
		return testkit.Case{
			Name:      "updateMerchantConnection " + body,
			Exchanges: exchange("updateMerchantConnection", 200, body, testkit.Example("updateMerchantConnection", 200)),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Update(ctx, params)
			},
			Check: merchantMatches(merchantInner(t, "updateMerchantConnection", 200, "connection")),
		}
	}
	get := func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantConnection.Get(ctx) }
	testkit.Run(t, []testkit.Case{
		{
			Name: "getMerchantConnection", Exchanges: exchange("getMerchantConnection", 200, "", testkit.Example("getMerchantConnection", 200)), Call: get,
			Check: func(t *testing.T, got any) {
				merchantMatches(connection)(t, got)
				c := got.(*ba.MerchantConnection)
				at := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
				if c.Provider != ba.MerchantProviderPolar || c.Environment != ba.MerchantEnvironmentSandbox ||
					c.ConfigVersion != 1 || !c.CreatedAt.Equal(at) || !c.UpdatedAt.Equal(at) ||
					!reflect.DeepEqual(c.Guard, ba.MerchantGuard{ProductIDs: []string{}}) {
					t.Fatalf("decoded %+v", c)
				}
			},
		},
		{
			Name: "getMerchantConnection unknown values and fields",
			Exchanges: exchange("getMerchantConnection", 200, "", `{"connection":{"provider":"stripe","environment":"test","futureField":1,`+
				`"guard":{"enabled":true,"productIds":["p1"],"meterId":"m1"}}}`),
			Call: get,
			Check: func(t *testing.T, got any) {
				c := got.(*ba.MerchantConnection)
				if c.Provider != "stripe" || c.Environment != "test" ||
					!reflect.DeepEqual(c.Guard, ba.MerchantGuard{Enabled: true, ProductIDs: []string{"p1"}, MeterID: new("m1")}) {
					t.Fatalf("decoded %+v", c)
				}
			},
		},
		{
			Name: "getMerchantConnection none", Exchanges: exchange("getMerchantConnection", 200, "", `{"connection":null}`), Call: get,
			Check: func(t *testing.T, got any) {
				if got.(*ba.MerchantConnection) != nil {
					t.Fatalf("got %+v, want nil", got)
				}
			},
		},
		{
			Name: "getMerchantConnection connection absent", Exchanges: exchange("getMerchantConnection", 200, "", `{}`), Call: get,
			Check: func(t *testing.T, got any) {
				if got.(*ba.MerchantConnection) != nil {
					t.Fatalf("got %+v, want nil", got)
				}
			},
		},
		{Name: "getMerchantConnection not an object", Exchanges: exchange("getMerchantConnection", 200, "", `[]`), Call: get,
			ErrPrefix: "blazingagents: getMerchantConnection: decode response: json: cannot unmarshal array into Go value of type map[string]"},
		{Name: "getMerchantConnection connection of the wrong type", Exchanges: exchange("getMerchantConnection", 200, "", `{"connection":{"configVersion":"1"}}`), Call: get,
			Err: "blazingagents: getMerchantConnection: decode response: json: cannot unmarshal string into Go struct field MerchantConnection.configVersion of type int64"},
		// A nil ProductIDs is required on the wire, so it is sent as []; a nil MeterID is null.
		create(ba.CreateMerchantConnectionParams{Provider: ba.MerchantProviderPolar, Environment: ba.MerchantEnvironmentSandbox,
			Credential: ba.NewSecret("polar_oat_123"), Guard: &ba.MerchantGuard{Enabled: true}},
			`{"provider":"polar","environment":"sandbox","credential":"polar_oat_123","guard":{"enabled":true,"productIds":[],"meterId":null}}`),
		create(ba.CreateMerchantConnectionParams{Provider: ba.MerchantProviderPolar, Environment: ba.MerchantEnvironmentSandbox,
			Credential: ba.NewSecret("polar_oat_123"), Guard: &ba.MerchantGuard{Enabled: true, ProductIDs: []string{}}},
			`{"provider":"polar","environment":"sandbox","credential":"polar_oat_123","guard":{"enabled":true,"productIds":[],"meterId":null}}`),
		create(ba.CreateMerchantConnectionParams{Provider: ba.MerchantProviderDodo, Environment: ba.MerchantEnvironmentLive,
			Credential: ba.NewSecret("dodo_key"), Guard: &ba.MerchantGuard{Enabled: true, ProductIDs: []string{"p1", "p2"}, MeterID: new("m1")}},
			`{"provider":"dodo","environment":"live","credential":"dodo_key","guard":{"enabled":true,"productIds":["p1","p2"],"meterId":"m1"}}`),
		create(ba.CreateMerchantConnectionParams{Provider: ba.MerchantProviderDodo, Environment: ba.MerchantEnvironmentLive, Credential: ba.NewSecret("k")},
			`{"provider":"dodo","environment":"live","credential":"k"}`),
		{
			Name:      "createMerchantConnection null",
			Exchanges: exchange("createMerchantConnection", 201, `{"provider":"dodo","environment":"live","credential":"k"}`, `{"connection":null}`),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Create(ctx, ba.CreateMerchantConnectionParams{Provider: "dodo", Environment: "live", Credential: ba.NewSecret("k")})
			},
			Err: `blazingagents: createMerchantConnection: decode response: "connection" is null`,
		},
		update(ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret("new-key")}, `{"credential":"new-key"}`),
		update(ba.UpdateMerchantConnectionParams{Guard: &ba.MerchantGuard{}}, `{"guard":{"enabled":false,"productIds":[],"meterId":null}}`),
		update(ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret(""),
			Guard: &ba.MerchantGuard{Enabled: true, MeterID: new("m1")}},
			`{"credential":"","guard":{"enabled":true,"productIds":[],"meterId":"m1"}}`),
		update(ba.UpdateMerchantConnectionParams{}, `{}`),
		{
			Name:      "updateMerchantConnection null",
			Exchanges: exchange("updateMerchantConnection", 200, `{}`, `{"connection":null}`),
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantConnection.Update(ctx, ba.UpdateMerchantConnectionParams{})
			},
			Err: `blazingagents: updateMerchantConnection: decode response: "connection" is null`,
		},
		{Name: "retireMerchantConnection", Exchanges: exchange("retireMerchantConnection", 204, "", ""),
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.MerchantConnection.Retire(ctx) }},
	})
}

// TestMerchantGuardReadModifyWrite sends a decoded guard back unchanged.
func TestMerchantGuardReadModifyWrite(t *testing.T) {
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "getMerchantConnection", Path: "/v1/merchant-connection", RespBody: testkit.Example("getMerchantConnection", 200)})
	m.Expect(testkit.Exchange{Op: "updateMerchantConnection", Path: "/v1/merchant-connection", Body: `{"guard":{"enabled":true,"productIds":[],"meterId":null}}`,
		RespBody: testkit.Example("updateMerchantConnection", 200)})
	ctx, c := context.Background(), m.Client()
	conn, err := c.MerchantConnection.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	guard := conn.Guard
	guard.Enabled = true
	if _, err := c.MerchantConnection.Update(ctx, ba.UpdateMerchantConnectionParams{Guard: &guard}); err != nil {
		t.Fatal(err)
	}
}

func TestMerchantBindings(t *testing.T) {
	const path = "/v1/merchant-connection/bindings"
	binding := `{"userId":"u1","customerId":"cus_1","createdAt":"2026-07-10T10:05:00.000Z","updatedAt":"2026-07-10T10:06:00.000Z"}`
	list := func(q url.Values, body string) testkit.Exchange {
		return testkit.Exchange{Op: "listMerchantBindings", Path: path, Query: q, Header: merchantTenant, RespBody: body}
	}
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listMerchantBindings",
			Exchanges: []testkit.Exchange{list(url.Values{"userId": {"app:user-42"}, "cursor": {"c1"}, "limit": {"200"}}, testkit.Example("listMerchantBindings", 200))},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.List(ctx, ba.ListMerchantBindingsParams{UserID: new("app:user-42"), Cursor: new("c1"), Limit: new(200)})
			},
			Check: func(t *testing.T, got any) {
				merchantMatches(testkit.Example("listMerchantBindings", 200))(t, got)
				l := got.(*ba.MerchantBindingList)
				if l.NextCursor != "" || l.Bindings[0].UserID != "app:user-42" || !l.Bindings[0].CreatedAt.Equal(time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC)) {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			// An empty user ID is still sent: it filters for the empty user rather than for none.
			Name:      "listMerchantBindings empty user",
			Exchanges: []testkit.Exchange{list(url.Values{"userId": {""}}, `{"bindings":[],"nextCursor":null}`)},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.List(ctx, ba.ListMerchantBindingsParams{UserID: new("")})
			},
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.MerchantBindingList); len(l.Bindings) != 0 || l.NextCursor != "" {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listMerchantBindings all",
			Exchanges: []testkit.Exchange{
				list(url.Values{"limit": {"1"}}, `{"bindings":[`+binding+`],"nextCursor":"c2"}`),
				list(url.Values{"limit": {"1"}, "cursor": {"c2"}}, `{"bindings":[{"userId":"u2"}],"nextCursor":null}`),
				list(url.Values{"limit": {"1"}}, `{"bindings":[`+binding+`],"nextCursor":"c2"}`),
				list(url.Values{"limit": {"1"}, "cursor": {"c2"}}, `{"bindings":[{"userId":"u2"}],"nextCursor":null}`),
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return merchantTwice(c.MerchantBindings.All(ctx, ba.ListMerchantBindingsParams{Limit: new(1)}))
			},
			Check: func(t *testing.T, got any) {
				var users []string
				for _, b := range got.([]ba.MerchantBinding) {
					users = append(users, b.UserID)
				}
				first := got.([]ba.MerchantBinding)[0]
				if !reflect.DeepEqual(users, []string{"u1", "u2", "u1", "u2"}) || first.CustomerID != "cus_1" ||
					!first.UpdatedAt.Equal(time.Date(2026, 7, 10, 10, 6, 0, 0, time.UTC)) {
					t.Fatalf("decoded %+v", got)
				}
			},
		},
		{
			Name: "listMerchantBindings all from a cursor stops at the error",
			Exchanges: []testkit.Exchange{
				list(url.Values{"cursor": {"c0"}}, `{"bindings":[`+binding+`],"nextCursor":"c2"}`),
				{Op: "listMerchantBindings", Path: path, Query: url.Values{"cursor": {"c2"}}, Status: 404, RespBody: `{"error":{"code":"merchant_connection_not_found","message":"Gone"}}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return merchantTwice(c.MerchantBindings.All(ctx, ba.ListMerchantBindingsParams{Cursor: new("c0")}))
			},
			Err: "blazingagents: listMerchantBindings: 404 merchant_connection_not_found: Gone",
		},
		{
			Name: "setMerchantBinding",
			Exchanges: []testkit.Exchange{{Op: "setMerchantBinding", Path: path + "/app%3Auser-42", Header: merchantTenant, Body: `{"customerId":"cus_1"}`,
				RespBody: testkit.Example("setMerchantBinding", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.Set(ctx, "app:user-42", ba.SetMerchantBindingParams{CustomerID: "cus_1"})
			},
			Check: merchantMatches(merchantInner(t, "setMerchantBinding", 200, "binding")),
		},
		{
			Name:      "setMerchantBinding null",
			Exchanges: []testkit.Exchange{{Op: "setMerchantBinding", Path: path + "/u1", Body: `{"customerId":"c"}`, RespBody: `{"binding":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.Set(ctx, "u1", ba.SetMerchantBindingParams{CustomerID: "c"})
			},
			Err: `blazingagents: setMerchantBinding: decode response: "binding" is null`,
		},
		{
			Name: "setMerchantBinding empty user",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantBindings.Set(ctx, "", ba.SetMerchantBindingParams{CustomerID: "c"})
			},
			Err: `blazingagents: invalid argument userId: "" is not a valid path segment`,
		},
		{Name: "deleteMerchantBinding", Exchanges: []testkit.Exchange{{Op: "deleteMerchantBinding", Path: path + "/a%20b%2F..", Header: merchantTenant, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return nil, c.MerchantBindings.Delete(ctx, "a b/..")
			}},
		{
			Name: "deleteMerchantBinding dot segment",
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.MerchantBindings.Delete(ctx, "..") },
			Err:  `blazingagents: invalid argument userId: ".." is not a valid path segment`,
		},
	})
}

func TestMerchantUsageEvents(t *testing.T) {
	const path = "/v1/merchant-usage-events"
	e1 := ba.MerchantUsageEventID(testkit.ID(op.IDPatterns["MerchantUsageEventID"]))
	e2 := ba.MerchantUsageEventID(testkit.ID(op.IDPatterns["MerchantUsageEventID"]))
	turn := ba.TurnID(testkit.ID(op.IDPatterns["TurnID"]))
	conn := ba.MerchantConnectionID(testkit.ID(op.IDPatterns["MerchantConnectionID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	eventPath := path + "/" + string(e1)
	eventExample := merchantInner(t, "getMerchantUsageEvent", 200, "event")
	// A discarded event, with every nullable field null, values this SDK does not list, and the removed workflowIssue.
	discarded := `{"id":"` + string(e2) + `","turnId":"` + string(turn) + `","connectionId":"` + string(conn) + `","provider":"stripe","userId":"u1",` +
		`"customerId":null,"agentId":"` + string(agent) + `","sessionId":null,"model":"m","modelProvider":"mistral","inputTokens":3000000000,` +
		`"outputTokens":1,"totalTokens":3000000001,"turnStatus":"timed_out","startedAt":"2026-07-10T10:00:00.000Z","occurredAt":"2026-07-10T10:00:01.000Z",` +
		`"status":"discarded","payload":null,"attemptCount":0,"attemptGeneration":2,"lastAttemptAt":null,"lastErrorCode":"MERCHANT_CUSTOMER_UNMAPPED",` +
		`"acceptedAt":null,"workflowIssue":"recovery_exhausted","nextAction":"none","futureField":true}`
	wantDiscarded := ba.MerchantUsageEvent{ID: e2, TurnID: turn, ConnectionID: conn, Provider: "stripe", UserID: "u1", AgentID: agent, Model: "m",
		ModelProvider: "mistral", InputTokens: 3000000000, OutputTokens: 1, TotalTokens: 3000000001, TurnStatus: "timed_out",
		StartedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC), OccurredAt: time.Date(2026, 7, 10, 10, 0, 1, 0, time.UTC),
		Status: ba.MerchantUsageEventStatusDiscarded, AttemptGeneration: 2, LastErrorCode: new("MERCHANT_CUSTOMER_UNMAPPED"),
		NextAction: ba.MerchantUsageEventActionNone}
	list := func(q url.Values, body string) testkit.Exchange {
		return testkit.Exchange{Op: "listMerchantUsageEvents", Path: path, Query: q, Header: merchantTenant, RespBody: body}
	}
	event := func(opID, suffix string, call func(context.Context, *ba.Client) (any, error)) testkit.Case {
		return testkit.Case{Name: opID, Exchanges: []testkit.Exchange{{Op: opID, Path: eventPath + suffix, Header: merchantTenant, RespBody: testkit.Example(opID, 200)}},
			Call: call, Check: merchantMatches(merchantInner(t, opID, 200, "event"))}
	}
	summary := merchantInner(t, "getMerchantUsageSummary", 200, "summary")
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listMerchantUsageEvents",
			Exchanges: []testkit.Exchange{list(url.Values{"status": {"failed"}, "cursor": {"c1"}, "limit": {"20"}}, testkit.Example("listMerchantUsageEvents", 200))},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.List(ctx, ba.ListMerchantUsageEventsParams{Status: new(ba.MerchantUsageEventStatusFailed), Cursor: new("c1"), Limit: new(20)})
			},
			Check: merchantMatches(testkit.Example("listMerchantUsageEvents", 200)),
		},
		{
			Name:      "listMerchantUsageEvents nulls and unknown values",
			Exchanges: []testkit.Exchange{list(nil, `{"events":[`+discarded+`],"nextCursor":"c2"}`)},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.List(ctx, ba.ListMerchantUsageEventsParams{})
			},
			Check: func(t *testing.T, got any) {
				if l := got.(*ba.MerchantUsageEventList); l.NextCursor != "c2" || !reflect.DeepEqual(l.Events, []ba.MerchantUsageEvent{wantDiscarded}) {
					t.Fatalf("decoded %+v", l)
				}
			},
		},
		{
			Name: "listMerchantUsageEvents all",
			Exchanges: []testkit.Exchange{
				list(url.Values{"status": {"discarded"}}, `{"events":[`+discarded+`],"nextCursor":"c2"}`),
				list(url.Values{"status": {"discarded"}, "cursor": {"c2"}}, `{"events":[{"id":"`+string(e1)+`"}],"nextCursor":null}`),
				list(url.Values{"status": {"discarded"}}, `{"events":[`+discarded+`],"nextCursor":"c2"}`),
				list(url.Values{"status": {"discarded"}, "cursor": {"c2"}}, `{"events":[{"id":"`+string(e1)+`"}],"nextCursor":null}`),
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return merchantTwice(c.MerchantUsageEvents.All(ctx, ba.ListMerchantUsageEventsParams{Status: new(ba.MerchantUsageEventStatusDiscarded)}))
			},
			Check: func(t *testing.T, got any) {
				var ids []ba.MerchantUsageEventID
				for _, e := range got.([]ba.MerchantUsageEvent) {
					ids = append(ids, e.ID)
				}
				if !reflect.DeepEqual(ids, []ba.MerchantUsageEventID{e2, e1, e2, e1}) {
					t.Fatalf("ids %v", ids)
				}
			},
		},
		{
			Name: "listMerchantUsageEvents all stops at the error",
			Exchanges: []testkit.Exchange{
				list(nil, `{"events":[`+discarded+`],"nextCursor":"c2"}`),
				{Op: "listMerchantUsageEvents", Path: path, Query: url.Values{"cursor": {"c2"}}, Status: 400, RespBody: `{"error":{"code":"invalid_cursor","message":"Bad cursor"}}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return merchantTwice(c.MerchantUsageEvents.All(ctx, ba.ListMerchantUsageEventsParams{}))
			},
			Err: "blazingagents: listMerchantUsageEvents: 400 invalid_cursor: Bad cursor",
		},
		{
			Name: "getMerchantUsageSummary",
			Exchanges: []testkit.Exchange{{Op: "getMerchantUsageSummary", Path: path + "/summary", Query: url.Values{"days": {"7"}}, Header: merchantTenant,
				RespBody: testkit.Example("getMerchantUsageSummary", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.Summary(ctx, ba.MerchantUsageSummaryParams{Days: new(7)})
			},
			Check: func(t *testing.T, got any) {
				merchantMatches(summary)(t, got)
				s := got.(*ba.MerchantUsageSummary)
				want := ba.MerchantUsageCounts{Pending: 2, Accepted: 318, Failed: 1, Discarded: 4}
				if s.Counts != want || !s.LastAcceptedAt.Equal(time.Date(2026, 7, 10, 10, 12, 10, 0, time.UTC)) ||
					!reflect.DeepEqual(s.Daily[0], ba.MerchantUsageDay{Day: ba.Date{Year: 2026, Month: time.July, Day: 9}, AcceptedEvents: 41, AcceptedTokens: 98430}) {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name: "getMerchantUsageSummary empty",
			Exchanges: []testkit.Exchange{{Op: "getMerchantUsageSummary", Path: path + "/summary",
				RespBody: `{"summary":{"counts":{},"lastAcceptedAt":null,"oldestPendingOccurredAt":null,"daily":[]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.MerchantUsageEvents.Summary(ctx, ba.MerchantUsageSummaryParams{})
			},
			Check: func(t *testing.T, got any) {
				if s := got.(*ba.MerchantUsageSummary); s.LastAcceptedAt != nil || s.OldestPendingOccurredAt != nil || s.Daily == nil || len(s.Daily) != 0 {
					t.Fatalf("decoded %+v", s)
				}
			},
		},
		{
			Name: "getMerchantUsageEvent", Exchanges: []testkit.Exchange{{Op: "getMerchantUsageEvent", Path: eventPath, RespBody: testkit.Example("getMerchantUsageEvent", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Get(ctx, e1) },
			Check: func(t *testing.T, got any) {
				merchantMatches(eventExample)(t, got)
				e := got.(*ba.MerchantUsageEvent)
				if e.Provider != ba.MerchantProviderPolar || e.ModelProvider != ba.ProviderTypeOpenRouter || e.TurnStatus != ba.MerchantTurnStatusSucceeded ||
					e.Status != ba.MerchantUsageEventStatusAccepted || e.NextAction != ba.MerchantUsageEventActionNone || e.TotalTokens != 2452 ||
					e.LastErrorCode != nil || *e.CustomerID == "" || e.SessionID == nil || e.Payload["events"] == nil ||
					!e.AcceptedAt.Equal(time.Date(2026, 7, 10, 10, 12, 10, 0, time.UTC)) {
					t.Fatalf("decoded %+v", e)
				}
			},
		},
		{
			Name: "getMerchantUsageEvent nulls", Exchanges: []testkit.Exchange{{Op: "getMerchantUsageEvent", Path: eventPath, RespBody: `{"event":` + discarded + `}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Get(ctx, e1) },
			Check: func(t *testing.T, got any) {
				if e := got.(*ba.MerchantUsageEvent); !reflect.DeepEqual(*e, wantDiscarded) {
					t.Fatalf("decoded %+v", e)
				}
			},
		},
		{
			Name: "getMerchantUsageEvent null", Exchanges: []testkit.Exchange{{Op: "getMerchantUsageEvent", Path: eventPath, RespBody: `{"event":null}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Get(ctx, e1) },
			Err:  `blazingagents: getMerchantUsageEvent: decode response: "event" is null`,
		},
		event("retryMerchantUsageEvent", "/retry", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Retry(ctx, e1) }),
		event("discardMerchantUsageEvent", "/discard", func(ctx context.Context, c *ba.Client) (any, error) { return c.MerchantUsageEvents.Discard(ctx, e1) }),
	})
}

// TestMerchantErrors sends one declared error code per operation.
func TestMerchantErrors(t *testing.T) {
	event := ba.MerchantUsageEventID(testkit.ID(op.IDPatterns["MerchantUsageEventID"]))
	const conn, bindings, events = "/v1/merchant-connection", "/v1/merchant-connection/bindings", "/v1/merchant-usage-events"
	eventPath := events + "/" + string(event)
	for _, tc := range []struct {
		e      testkit.Exchange
		status int
		code   ba.ErrorCode
		call   func(context.Context, *ba.Client) error
	}{
		{testkit.Exchange{Op: "getMerchantConnection", Path: conn}, 403, ba.ErrorCodeForbidden,
			func(ctx context.Context, c *ba.Client) error { return merchantErr(c.MerchantConnection.Get(ctx)) }},
		{testkit.Exchange{Op: "createMerchantConnection", Path: conn, Body: `{"provider":"polar","environment":"live","credential":"bad"}`}, 422, ba.ErrorCodeMerchantCredentialInvalid,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantConnection.Create(ctx, ba.CreateMerchantConnectionParams{Provider: "polar", Environment: "live", Credential: ba.NewSecret("bad")}))
			}},
		{testkit.Exchange{Op: "updateMerchantConnection", Path: conn, Body: `{"credential":"other"}`}, 409, ba.ErrorCodeMerchantAccountMismatch,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantConnection.Update(ctx, ba.UpdateMerchantConnectionParams{Credential: ba.NewSecret("other")}))
			}},
		{testkit.Exchange{Op: "retireMerchantConnection", Path: conn}, 404, ba.ErrorCodeMerchantConnectionNotFound,
			func(ctx context.Context, c *ba.Client) error { return c.MerchantConnection.Retire(ctx) }},
		{testkit.Exchange{Op: "listMerchantBindings", Path: bindings, Query: url.Values{"cursor": {"x"}}}, 400, ba.ErrorCodeInvalidCursor,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantBindings.List(ctx, ba.ListMerchantBindingsParams{Cursor: new("x")}))
			}},
		{testkit.Exchange{Op: "setMerchantBinding", Path: bindings + "/u1", Body: `{"customerId":"missing"}`}, 422, ba.ErrorCodeMerchantCustomerNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantBindings.Set(ctx, "u1", ba.SetMerchantBindingParams{CustomerID: "missing"}))
			}},
		{testkit.Exchange{Op: "deleteMerchantBinding", Path: bindings + "/u1"}, 404, ba.ErrorCodeMerchantBindingNotFound,
			func(ctx context.Context, c *ba.Client) error { return c.MerchantBindings.Delete(ctx, "u1") }},
		{testkit.Exchange{Op: "listMerchantUsageEvents", Path: events}, 402, ba.ErrorCodeSubscriptionRequired,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantUsageEvents.List(ctx, ba.ListMerchantUsageEventsParams{}))
			}},
		{testkit.Exchange{Op: "getMerchantUsageSummary", Path: events + "/summary", Query: url.Values{"days": {"91"}}}, 400, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantUsageEvents.Summary(ctx, ba.MerchantUsageSummaryParams{Days: new(91)}))
			}},
		{testkit.Exchange{Op: "getMerchantUsageEvent", Path: eventPath}, 404, ba.ErrorCodeMerchantEventNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantUsageEvents.Get(ctx, event))
			}},
		{testkit.Exchange{Op: "retryMerchantUsageEvent", Path: eventPath + "/retry"}, 409, ba.ErrorCodeMerchantEventStateConflict,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantUsageEvents.Retry(ctx, event))
			}},
		{testkit.Exchange{Op: "discardMerchantUsageEvent", Path: eventPath + "/discard"}, 403, ba.ErrorCodeInsufficientScope,
			func(ctx context.Context, c *ba.Client) error {
				return merchantErr(c.MerchantUsageEvents.Discard(ctx, event))
			}},
	} {
		t.Run(tc.e.Op, func(t *testing.T) {
			m := testkit.New(t)
			tc.e.Status = tc.status
			tc.e.RespHeader = http.Header{"X-Request-Id": {"req_err"}}
			tc.e.RespBody = `{"error":{"code":"` + string(tc.code) + `","message":"It failed","param":"name","details":{"n":1}}}`
			m.Expect(tc.e)
			err := tc.call(context.Background(), m.Client())
			want := "blazingagents: " + tc.e.Op + ": " + strconv.Itoa(tc.status) + " " + string(tc.code) + ": It failed (request req_err)"
			var apiErr *ba.APIError
			if !errors.As(err, &apiErr) || err.Error() != want {
				t.Fatalf("error %v, want %s", err, want)
			}
			if apiErr.Status != tc.status || apiErr.Code != tc.code || apiErr.Message != "It failed" || apiErr.Param != "name" ||
				!reflect.DeepEqual(apiErr.Details, map[string]any{"n": float64(1)}) || apiErr.Meta.OperationID != tc.e.Op ||
				apiErr.Meta.Status != tc.status || apiErr.Meta.RequestID != "req_err" {
				t.Fatalf("APIError %#v", apiErr)
			}
		})
	}
}

// TestMerchantNullConnectionIsADecodeError checks the error type behind the "is null" message.
func TestMerchantNullConnectionIsADecodeError(t *testing.T) {
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "updateMerchantConnection", Path: "/v1/merchant-connection", Body: `{}`,
		RespHeader: http.Header{"X-Request-Id": {"req_null"}}, RespBody: `{"connection":null}`})
	got, err := m.Client().MerchantConnection.Update(context.Background(), ba.UpdateMerchantConnectionParams{})
	var de *ba.DecodeError
	if got != nil || !errors.As(err, &de) || de.OperationID != "updateMerchantConnection" || de.RequestID != "req_null" {
		t.Fatalf("got %v, %#v", got, err)
	}
}
