package blazingagents_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	ba "github.com/blazingagents/golang-sdk"
	"github.com/blazingagents/golang-sdk/internal/op"
	"github.com/blazingagents/golang-sdk/internal/testkit"
)

var (
	_ = testkit.RedactionCase("SlackCredentials", func(secret string) any {
		return ba.SlackCredentials{BotToken: ba.NewSecret(secret), SigningSecret: ba.NewSecret(secret)}
	})
	_ = testkit.RedactionCase("TelegramCredentials", func(secret string) any { return ba.TelegramCredentials{BotToken: ba.NewSecret(secret)} })
	_ = testkit.RedactionCase("SlackConnection", func(secret string) any {
		return ba.SlackConnection{Name: "n", Credentials: ba.SlackCredentials{BotToken: ba.NewSecret(secret), SigningSecret: ba.NewSecret(secret)}}
	})
	_ = testkit.RedactionCase("TelegramConnection", func(secret string) any {
		return &ba.TelegramConnection{Name: "n", Credentials: ba.TelegramCredentials{BotToken: ba.NewSecret(secret)}}
	})
)

// chatConnectionsTenant is what every request of these tenant-only operations must carry.
var chatConnectionsTenant = http.Header{"Authorization": {"Bearer " + testkit.APIKey}}

func chatConnectionsErr[T any](_ T, err error) error { return err }

// chatConnectionsListed wraps a decoded list in its envelope, to compare it with a response body.
func chatConnectionsListed(got any) any { return map[string]any{"chatConnections": got} }

func chatConnectionsCovers(example string) func(*testing.T, any) {
	return func(t *testing.T, got any) { testkit.CoversExample(t, example, got) }
}

func TestChatConnections(t *testing.T) {
	conn := ba.ChatConnectionID(testkit.ID(op.IDPatterns["ChatConnectionID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	path := "/v1/chat-connections/" + string(conn)
	example := testkit.Example("getChatConnection", 200)
	withConfig := func(config string) string { return `{"id":"` + string(conn) + `","configuration":` + config + `}` }
	get := func(body string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "getChatConnection", Path: path, Header: chatConnectionsTenant, NoHeader: []string{"X-BA-User-Id"}, RespBody: body}}
	}
	getCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Get(ctx, conn) }
	config := func(want ba.ChatConfiguration) func(*testing.T, any) {
		return func(t *testing.T, got any) {
			if c := got.(*ba.ChatConnection).Configuration; !reflect.DeepEqual(c, want) {
				t.Fatalf("configuration %#v, want %#v", c, want)
			}
		}
	}
	create := func(spec ba.ChatConnectionSpec, body string) testkit.Case {
		return testkit.Case{
			Name: "createChatConnection " + body,
			Exchanges: []testkit.Exchange{{Op: "createChatConnection", Path: "/v1/chat-connections", Body: body, Status: 201,
				Header: http.Header{"Content-Type": {"application/json"}}, NoHeader: []string{"X-BA-User-Id"}, RespBody: testkit.Example("createChatConnection", 201)}},
			Call:    func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Create(ctx, spec) },
			Example: testkit.Example("createChatConnection", 201),
			Check:   chatConnectionsCovers(testkit.Example("createChatConnection", 201)),
		}
	}
	rotate := func(creds ba.ChatCredentials, body string) testkit.Case {
		return testkit.Case{
			Name:      "rotateChatCredentials " + body,
			Exchanges: []testkit.Exchange{{Op: "rotateChatCredentials", Path: path + "/credentials", Body: body, RespBody: testkit.Example("rotateChatCredentials", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.RotateCredentials(ctx, conn, creds)
			},
			Example: testkit.Example("rotateChatCredentials", 200),
			Check:   chatConnectionsCovers(testkit.Example("rotateChatCredentials", 200)),
		}
	}
	rename := func(params ba.UpdateChatConnectionParams, body string) testkit.Case {
		return testkit.Case{
			Name:      "renameChatConnection " + body,
			Exchanges: []testkit.Exchange{{Op: "renameChatConnection", Path: path, Body: body, RespBody: testkit.Example("renameChatConnection", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.Update(ctx, conn, params)
			},
			Example: testkit.Example("renameChatConnection", 200),
		}
	}
	action := func(opID, suffix string, call func(context.Context, *ba.Client) (any, error)) testkit.Case {
		return testkit.Case{Name: opID, Exchanges: []testkit.Exchange{{Op: opID, Path: path + suffix, RespBody: testkit.Example(opID, 200)}},
			Call: call, Example: testkit.Example(opID, 200), Check: chatConnectionsCovers(testkit.Example(opID, 200))}
	}
	unknownList := testkit.Exchange{Op: "listChatConnections", Path: "/v1/chat-connections",
		RespBody: `{"chatConnections":[` + example + `,` + withConfig(`{"platform":"discord","guildId":"g"}`) + `]}`}
	slack := ba.SlackCredentials{BotToken: ba.NewSecret("xoxb-1-2"), SigningSecret: ba.NewSecret("0123456789abcdef0123456789abcdef")}
	telegram := ba.TelegramCredentials{BotToken: ba.NewSecret("123:abc")}
	testkit.Run(t, []testkit.Case{
		{
			Name: "listChatConnections",
			Exchanges: []testkit.Exchange{{Op: "listChatConnections", Path: "/v1/chat-connections", Header: chatConnectionsTenant, NoHeader: []string{"X-BA-User-Id"},
				RespBody: testkit.Example("listChatConnections", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.List(ctx) },
			Check: func(t *testing.T, got any) {
				testkit.MatchesExample(t, testkit.Example("listChatConnections", 200), chatConnectionsListed(got))
				testkit.CoversExample(t, testkit.Example("listChatConnections", 200), chatConnectionsListed(got))
			},
		},
		{
			Name:      "listChatConnections unknown platform",
			Exchanges: []testkit.Exchange{unknownList},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.List(ctx) },
			Check:     func(t *testing.T, got any) { testkit.UnknownRoundTrip(t, unknownList, chatConnectionsListed(got)) },
		},
		{
			Name:      "listChatConnections empty",
			Exchanges: []testkit.Exchange{{Op: "listChatConnections", Path: "/v1/chat-connections", RespBody: `{"chatConnections":[]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.List(ctx) },
			Check: func(t *testing.T, got any) {
				if l := got.([]ba.ChatConnection); l == nil || len(l) != 0 {
					t.Fatalf("got %#v, want an empty list", l)
				}
			},
		},
		// The expected bodies follow the spec: configuration and credentials are nested, and
		// credentials carry no platform.
		create(ba.SlackConnection{AgentID: agent, Name: "Support", Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Support","platform":"slack","credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		create(ba.SlackConnection{AgentID: agent, Name: "Support", Enabled: new(false), ChannelIDs: []string{"C1"}, Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Support","enabled":false,"platform":"slack","configuration":{"channelIds":["C1"]},"credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		create(ba.SlackConnection{AgentID: agent, Name: "Support", Enabled: new(true), ChannelIDs: []string{}, Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Support","enabled":true,"platform":"slack","configuration":{"channelIds":[]},"credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","credentials":{"botToken":"123:abc"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", ChatIDs: []string{"-1"}, BusinessMode: new(true), Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","configuration":{"chatIds":["-1"],"businessMode":true},"credentials":{"botToken":"123:abc"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", BusinessMode: new(false), Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","configuration":{"businessMode":false},"credentials":{"botToken":"123:abc"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", ChatIDs: []string{}, Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","configuration":{"chatIds":[]},"credentials":{"botToken":"123:abc"}}`),
		create(&ba.SlackConnection{AgentID: agent, Name: "Pointer", Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Pointer","platform":"slack","credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		{
			Name: "createChatConnection nil",
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Create(ctx, nil) },
			Err:  "blazingagents: invalid argument params: nil",
		},
		{
			Name: "createChatConnection typed nil pointer",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var spec *ba.SlackConnection
				return c.ChatConnections.Create(ctx, spec)
			},
			Err: "blazingagents: invalid argument params: nil",
		},
		{
			Name: "getChatConnection", Exchanges: get(example), Call: getCall, Example: example,
			Check: func(t *testing.T, got any) {
				testkit.CoversExample(t, example, got)
				c := got.(*ba.ChatConnection)
				at := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
				want := ba.ChatHealth{CheckedAt: at, TokenValid: true, IdentityVerified: true, Checks: []ba.ChatHealthCheck{
					{Code: "bot_identity", Status: ba.ChatHealthCheckStatusPass},
					{Code: "channel_membership", Status: ba.ChatHealthCheckStatusUnknown},
					{Code: "webhook_url", Status: ba.ChatHealthCheckStatusPass},
				}}
				if c.Platform != ba.ChatPlatformTelegram || !c.Enabled || c.CredentialVersion != 1 || !c.CreatedAt.Equal(at) || !c.UpdatedAt.Equal(at) ||
					c.Identity.TeamID != nil || c.Identity.AppID != nil || !reflect.DeepEqual(c.Health, want) ||
					!reflect.DeepEqual(c.Configuration, ba.TelegramConfiguration{ChatIDs: []string{}}) {
					t.Fatalf("decoded %+v", c)
				}
			},
		},
		{
			Name: "getChatConnection slack with unknown values and fields",
			Exchanges: get(`{"id":"` + string(conn) + `","platform":"discord","enabled":false,"futureField":{"x":1},` +
				`"configuration":{"platform":"slack","channelIds":["C1"]},"identity":{"botId":"B","botUserId":"U","teamId":"T","appId":"A"},` +
				`"health":{"checkedAt":"2026-07-10T10:00:00.5+01:00","tokenValid":false,"identityVerified":false,"checks":[{"code":"scope","status":"degraded","subject":"chat:write"}]}}`),
			Call: getCall,
			Check: func(t *testing.T, got any) {
				config(ba.SlackConfiguration{ChannelIDs: []string{"C1"}})(t, got)
				c := got.(*ba.ChatConnection)
				if c.Platform != "discord" || *c.Identity.TeamID != "T" || *c.Identity.AppID != "A" ||
					!c.Health.CheckedAt.Equal(time.Date(2026, 7, 10, 9, 0, 0, 5e8, time.UTC)) ||
					!reflect.DeepEqual(c.Health.Checks, []ba.ChatHealthCheck{{Code: "scope", Status: "degraded", Subject: new("chat:write")}}) {
					t.Fatalf("decoded %+v", c)
				}
			},
		},
		{Name: "getChatConnection telegram", Exchanges: get(withConfig(`{"platform":"telegram","chatIds":["-1","2"],"businessMode":true}`)), Call: getCall,
			Check: config(ba.TelegramConfiguration{ChatIDs: []string{"-1", "2"}, BusinessMode: true})},
		{Name: "getChatConnection slack without channels", Exchanges: get(withConfig(`{"platform":"slack"}`)), Call: getCall,
			Check: config(ba.SlackConfiguration{})},
		{Name: "getChatConnection unknown platform", Exchanges: get(withConfig(`{"platform":"discord","guildId":"g"}`)), Call: getCall, Unknown: true,
			Check: config(ba.UnknownChatConfiguration{Platform: "discord", Raw: []byte(`{"platform":"discord","guildId":"g"}`)})},
		{Name: "getChatConnection configuration null", Exchanges: get(withConfig(`null`)), Call: getCall, Check: config(nil)},
		{Name: "getChatConnection platform missing", Exchanges: get(withConfig(`{"channelIds":["C1"]}`)), Call: getCall,
			Err: `blazingagents: getChatConnection: decode response: ChatConfiguration: missing "platform"`},
		{Name: "getChatConnection platform null", Exchanges: get(withConfig(`{"platform":null}`)), Call: getCall,
			Err: `blazingagents: getChatConnection: decode response: ChatConfiguration: "platform" is null`},
		{Name: "getChatConnection known platform malformed", Exchanges: get(withConfig(`{"platform":"slack","channelIds":"C1"}`)), Call: getCall,
			Err: "blazingagents: getChatConnection: decode response: json: cannot unmarshal string into Go struct field slackConfigurationWire.channelIds of type []string"},
		{Name: "getChatConnection host field of the wrong type", Exchanges: get(`{"id":false}`), Call: getCall,
			ErrPrefix: "blazingagents: getChatConnection: decode response: json: "},
		rename(ba.UpdateChatConnectionParams{Name: new("Renamed"), Configuration: &ba.ChatConfigurationUpdate{ChatIDs: []string{"-2"}}},
			`{"name":"Renamed","configuration":{"chatIds":["-2"]}}`),
		rename(ba.UpdateChatConnectionParams{Name: new("Renamed")}, `{"name":"Renamed"}`),
		rename(ba.UpdateChatConnectionParams{Configuration: &ba.ChatConfigurationUpdate{ChannelIDs: []string{"C1", "G2"}}}, `{"configuration":{"channelIds":["C1","G2"]}}`),
		rename(ba.UpdateChatConnectionParams{Configuration: &ba.ChatConfigurationUpdate{ChannelIDs: []string{}}}, `{"configuration":{"channelIds":[]}}`),
		rename(ba.UpdateChatConnectionParams{Configuration: &ba.ChatConfigurationUpdate{BusinessMode: new(false), ChatIDs: []string{}}},
			`{"configuration":{"businessMode":false,"chatIds":[]}}`),
		rename(ba.UpdateChatConnectionParams{Configuration: &ba.ChatConfigurationUpdate{}}, `{"configuration":{}}`),
		rename(ba.UpdateChatConnectionParams{}, `{}`),
		{Name: "deleteChatConnection", Exchanges: []testkit.Exchange{{Op: "deleteChatConnection", Path: path, Header: chatConnectionsTenant, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.ChatConnections.Delete(ctx, conn) }},
		action("enableChatConnection", "/enable", func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Enable(ctx, conn) }),
		action("disableChatConnection", "/disable", func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Disable(ctx, conn) }),
		action("checkChatHealth", "/health", func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.CheckHealth(ctx, conn) }),
		// rotateChatCredentials is flat, with platform: the one place credentials carry it.
		rotate(slack, `{"platform":"slack","botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}`),
		rotate(telegram, `{"platform":"telegram","botToken":"123:abc"}`),
		rotate(&telegram, `{"platform":"telegram","botToken":"123:abc"}`),
		{
			Name: "rotateChatCredentials nil",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.RotateCredentials(ctx, conn, nil)
			},
			Err: "blazingagents: invalid argument params: nil",
		},
		{
			Name: "rotateChatCredentials typed nil pointer",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var creds *ba.TelegramCredentials
				return c.ChatConnections.RotateCredentials(ctx, conn, creds)
			},
			Err: "blazingagents: invalid argument params: nil",
		},
	})
}

// TestChatConnectionsDecodeError checks the type and fields of a malformed known variant's error.
func TestChatConnectionsDecodeError(t *testing.T) {
	conn := ba.ChatConnectionID(testkit.ID(op.IDPatterns["ChatConnectionID"]))
	m := testkit.New(t)
	m.Expect(testkit.Exchange{Op: "checkChatHealth", Path: "/v1/chat-connections/" + string(conn) + "/health",
		RespHeader: http.Header{"X-Request-Id": {"req_dec"}}, RespBody: `{"configuration":{"platform":"telegram","businessMode":"yes"}}`})
	got, err := m.Client().ChatConnections.CheckHealth(context.Background(), conn)
	var de *ba.DecodeError
	if got != nil || !errors.As(err, &de) || de.OperationID != "checkChatHealth" || de.RequestID != "req_dec" || de.Err == nil {
		t.Fatalf("got %v, %#v", got, err)
	}
}

// TestChatConnectionsErrors sends one declared error code per operation.
func TestChatConnectionsErrors(t *testing.T) {
	conn := ba.ChatConnectionID(testkit.ID(op.IDPatterns["ChatConnectionID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	path := "/v1/chat-connections/" + string(conn)
	for _, tc := range []struct {
		e      testkit.Exchange
		status int
		code   ba.ErrorCode
		call   func(context.Context, *ba.Client) error
	}{
		{testkit.Exchange{Op: "listChatConnections", Path: "/v1/chat-connections"}, 402, ba.ErrorCodeSubscriptionRequired,
			func(ctx context.Context, c *ba.Client) error { return chatConnectionsErr(c.ChatConnections.List(ctx)) }},
		{testkit.Exchange{Op: "createChatConnection", Path: "/v1/chat-connections",
			Body: `{"agentId":"` + string(agent) + `","name":"n","platform":"telegram","credentials":{"botToken":"1:a"}}`}, 409, ba.ErrorCodeAdminAgentManaged,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.Create(ctx, ba.TelegramConnection{AgentID: agent, Name: "n",
					Credentials: ba.TelegramCredentials{BotToken: ba.NewSecret("1:a")}}))
			}},
		{testkit.Exchange{Op: "getChatConnection", Path: path}, 404, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.Get(ctx, conn))
			}},
		{testkit.Exchange{Op: "renameChatConnection", Path: path, Body: `{"configuration":{"businessMode":true}}`}, 502, ba.ErrorCodeChatWebhookRegistrationFailed,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.Update(ctx, conn, ba.UpdateChatConnectionParams{Configuration: &ba.ChatConfigurationUpdate{BusinessMode: new(true)}}))
			}},
		{testkit.Exchange{Op: "deleteChatConnection", Path: path}, 403, ba.ErrorCodeInsufficientScope,
			func(ctx context.Context, c *ba.Client) error { return c.ChatConnections.Delete(ctx, conn) }},
		{testkit.Exchange{Op: "enableChatConnection", Path: path + "/enable"}, 409, ba.ErrorCodeChatWebhookConflict,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.Enable(ctx, conn))
			}},
		{testkit.Exchange{Op: "disableChatConnection", Path: path + "/disable"}, 404, ba.ErrorCodeNotFound,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.Disable(ctx, conn))
			}},
		{testkit.Exchange{Op: "checkChatHealth", Path: path + "/health"}, 401, ba.ErrorCodeUnauthorized,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.CheckHealth(ctx, conn))
			}},
		{testkit.Exchange{Op: "rotateChatCredentials", Path: path + "/credentials", Body: `{"platform":"telegram","botToken":"1:a"}`}, 400, ba.ErrorCodeValidationFailed,
			func(ctx context.Context, c *ba.Client) error {
				return chatConnectionsErr(c.ChatConnections.RotateCredentials(ctx, conn, ba.TelegramCredentials{BotToken: ba.NewSecret("1:a")}))
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
