package blazingagents_test

import (
	"context"
	"net/url"
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

func TestChatConnectionsSmoke(t *testing.T) {
	conn := ba.ChatConnectionID(testkit.ID(op.IDPatterns["ChatConnectionID"]))
	delivery := ba.ChatDeliveryID(testkit.ID(op.IDPatterns["ChatDeliveryID"]))
	agent := ba.AgentID(testkit.ID(op.IDPatterns["AgentID"]))
	path := "/v1/chat-connections/" + string(conn)
	example := testkit.Example("getChatConnection", 200)
	withConfig := func(config string) string { return `{"id":"` + string(conn) + `","configuration":` + config + `}` }
	get := func(body string) []testkit.Exchange {
		return []testkit.Exchange{{Op: "getChatConnection", Path: path, RespBody: body}}
	}
	getCall := func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Get(ctx, conn) }
	config := func(want ba.ChatConfiguration) func(*testing.T, any) {
		return func(t *testing.T, got any) {
			if c := got.(*ba.ChatConnection).Configuration; jsonString(c) != jsonString(want) {
				t.Fatalf("configuration %#v, want %#v", c, want)
			}
		}
	}
	create := func(spec ba.ChatConnectionSpec, body string) testkit.Case {
		return testkit.Case{
			Name:      "createChatConnection " + body,
			Exchanges: []testkit.Exchange{{Op: "createChatConnection", Path: "/v1/chat-connections", Body: body, Status: 201, RespBody: testkit.Example("createChatConnection", 201)}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Create(ctx, spec) },
			Example:   testkit.Example("createChatConnection", 201),
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
		}
	}
	slack := ba.SlackCredentials{BotToken: ba.NewSecret("xoxb-1-2"), SigningSecret: ba.NewSecret("0123456789abcdef0123456789abcdef")}
	telegram := ba.TelegramCredentials{BotToken: ba.NewSecret("123:abc")}
	deliveries := `{"data":[{"id":"` + string(delivery) + `"}],"nextCursor":"c2"}`
	testkit.Run(t, []testkit.Case{
		{
			Name:      "listChatConnections",
			Exchanges: []testkit.Exchange{{Op: "listChatConnections", Path: "/v1/chat-connections", RespBody: `{"chatConnections":[` + example + `]}`}},
			Call:      func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.List(ctx) },
			Example:   `[` + example + `]`,
		},
		// The expected bodies follow the spec: configuration and credentials are nested, and
		// credentials carry no platform.
		create(ba.SlackConnection{AgentID: agent, Name: "Support", Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Support","platform":"slack","credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		create(ba.SlackConnection{AgentID: agent, Name: "Support", Enabled: new(false), ChannelIDs: []string{"C1"}, Credentials: slack},
			`{"agentId":"`+string(agent)+`","name":"Support","enabled":false,"platform":"slack","configuration":{"channelIds":["C1"]},"credentials":{"botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","credentials":{"botToken":"123:abc"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", ChatIDs: []string{"-1"}, BusinessMode: new(true), Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","configuration":{"chatIds":["-1"],"businessMode":true},"credentials":{"botToken":"123:abc"}}`),
		create(ba.TelegramConnection{AgentID: agent, Name: "Bot", BusinessMode: new(false), Credentials: telegram},
			`{"agentId":"`+string(agent)+`","name":"Bot","platform":"telegram","configuration":{"businessMode":false},"credentials":{"botToken":"123:abc"}}`),
		{
			Name: "createChatConnection nil",
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Create(ctx, nil) },
			Err:  "blazingagents: invalid argument params: nil",
		},
		{Name: "getChatConnection", Exchanges: get(example), Call: getCall, Example: example},
		{Name: "getChatConnection slack", Exchanges: get(withConfig(`{"platform":"slack","channelIds":["C1"]}`)), Call: getCall,
			Check: config(ba.SlackConfiguration{ChannelIDs: []string{"C1"}})},
		{Name: "getChatConnection unknown platform", Exchanges: get(withConfig(`{"platform":"discord","guildId":"g"}`)), Call: getCall, Unknown: true},
		{Name: "getChatConnection host field of the wrong type", Exchanges: get(`{"id":false}`), Call: getCall,
			ErrPrefix: "blazingagents: getChatConnection: decode response: json: "},
		{
			Name: "renameChatConnection",
			Exchanges: []testkit.Exchange{{Op: "renameChatConnection", Path: path, RespBody: testkit.Example("renameChatConnection", 200),
				Body: `{"name":"Renamed","configuration":{"chatIds":["-2"]}}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.Update(ctx, conn, ba.UpdateChatConnectionParams{Name: new("Renamed"), Configuration: &ba.ChatConfigurationUpdate{ChatIDs: []string{"-2"}}})
			},
			Example: testkit.Example("renameChatConnection", 200),
		},
		{Name: "deleteChatConnection", Exchanges: []testkit.Exchange{{Op: "deleteChatConnection", Path: path, Status: 204}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return nil, c.ChatConnections.Delete(ctx, conn) }},
		{Name: "enableChatConnection", Exchanges: []testkit.Exchange{{Op: "enableChatConnection", Path: path + "/enable", RespBody: example}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Enable(ctx, conn) }, Example: example},
		{Name: "disableChatConnection", Exchanges: []testkit.Exchange{{Op: "disableChatConnection", Path: path + "/disable", RespBody: example}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.Disable(ctx, conn) }, Example: example},
		{Name: "checkChatHealth", Exchanges: []testkit.Exchange{{Op: "checkChatHealth", Path: path + "/health", RespBody: testkit.Example("checkChatHealth", 200)}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) { return c.ChatConnections.CheckHealth(ctx, conn) }, Example: testkit.Example("checkChatHealth", 200)},
		// rotateChatCredentials is flat, with platform: the one place credentials carry it.
		rotate(slack, `{"platform":"slack","botToken":"xoxb-1-2","signingSecret":"0123456789abcdef0123456789abcdef"}`),
		rotate(telegram, `{"platform":"telegram","botToken":"123:abc"}`),
		{
			Name: "rotateChatCredentials nil",
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.RotateCredentials(ctx, conn, nil)
			},
			Err: "blazingagents: invalid argument params: nil",
		},
		{
			Name:      "listChatDeliveries",
			Exchanges: []testkit.Exchange{{Op: "listChatDeliveries", Path: path + "/deliveries", Query: url.Values{"limit": {"10"}}, RespBody: deliveries}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.ListDeliveries(ctx, conn, ba.ListChatDeliveriesParams{Limit: new(10)})
			},
			Example: deliveries,
		},
		{
			Name: "listChatDeliveries all",
			Exchanges: []testkit.Exchange{
				{Op: "listChatDeliveries", Path: path + "/deliveries", RespBody: deliveries},
				{Op: "listChatDeliveries", Path: path + "/deliveries", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var ids []ba.ChatDeliveryID
				for d, err := range c.ChatConnections.AllDeliveries(ctx, conn, ba.ListChatDeliveriesParams{}) {
					if err != nil {
						return nil, err
					}
					ids = append(ids, d.ID)
				}
				return ids, nil
			},
			Example: `["` + string(delivery) + `"]`,
		},
		{
			Name: "repairChatDelivery",
			Exchanges: []testkit.Exchange{{Op: "repairChatDelivery", Path: path + "/deliveries/" + string(delivery) + "/repair", RespBody: testkit.Example("repairChatDelivery", 200),
				Body: `{"expectedAttempt":0,"previousSenderStopped":true,"acceptDuplicateRisk":true}`}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatConnections.RepairDelivery(ctx, conn, delivery, ba.RepairChatDeliveryParams{})
			},
			Example: testkit.Example("repairChatDelivery", 200),
		},
		{
			Name: "listTenantChatDeliveries",
			Exchanges: []testkit.Exchange{{Op: "listTenantChatDeliveries", Path: "/v1/chat-deliveries", RespBody: deliveries,
				Query: url.Values{"status": {"failed,ambiguous"}, "since": {"2026-07-10T10:00:00.5Z"}}}},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				return c.ChatDeliveries.List(ctx, ba.ListTenantChatDeliveriesParams{
					Status: []ba.ChatDeliveryStatus{ba.ChatDeliveryStatusFailed, ba.ChatDeliveryStatusAmbiguous},
					Since:  new(time.Date(2026, 7, 10, 11, 0, 0, 5e8, time.FixedZone("CET", 3600)))})
			},
			Example: deliveries,
		},
		{
			Name: "listTenantChatDeliveries all",
			Exchanges: []testkit.Exchange{
				{Op: "listTenantChatDeliveries", Path: "/v1/chat-deliveries", RespBody: deliveries},
				{Op: "listTenantChatDeliveries", Path: "/v1/chat-deliveries", Query: url.Values{"cursor": {"c2"}}, RespBody: `{"data":[],"nextCursor":null}`},
			},
			Call: func(ctx context.Context, c *ba.Client) (any, error) {
				var n int
				for _, err := range c.ChatDeliveries.All(ctx, ba.ListTenantChatDeliveriesParams{}) {
					if err != nil {
						return nil, err
					}
					n++
				}
				return n, nil
			},
			Example: `1`,
		},
	})
}
