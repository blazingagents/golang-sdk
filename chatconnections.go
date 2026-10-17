package blazingagents

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ChatConnection is an agent's connection to a Slack workspace or a Telegram bot. Its
// credentials are never returned; CredentialFragment holds the token's last four characters.
// See https://docs.blazingagents.com/platform/chat-integrations.
type ChatConnection struct {
	ID                 ChatConnectionID  `json:"id"`
	TenantID           TenantID          `json:"tenantId"`
	AgentID            AgentID           `json:"agentId"`
	Name               string            `json:"name"`
	Platform           ChatPlatform      `json:"platform"`
	Enabled            bool              `json:"enabled"`
	Configuration      ChatConfiguration `json:"configuration"`
	Identity           ChatIdentity      `json:"identity"`
	Health             ChatHealth        `json:"health"`
	CredentialFragment string            `json:"credentialFragment"`
	CredentialVersion  int64             `json:"credentialVersion"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	WebhookURL         string            `json:"webhookUrl"`
}

// ChatIdentity is the bot a connection posts as. TeamID and AppID are nil on Telegram.
type ChatIdentity struct {
	BotID     string  `json:"botId"`
	BotUserID string  `json:"botUserId"`
	TeamID    *string `json:"teamId"`
	AppID     *string `json:"appId"`
}

// ChatHealth is the result of a connection's latest health check.
type ChatHealth struct {
	CheckedAt        time.Time         `json:"checkedAt"`
	TokenValid       bool              `json:"tokenValid"`
	IdentityVerified bool              `json:"identityVerified"`
	Checks           []ChatHealthCheck `json:"checks"`
}

// ChatHealthCheck is one check of a health check, such as bot_identity or webhook_url.
type ChatHealthCheck struct {
	Code    string                `json:"code"`
	Status  ChatHealthCheckStatus `json:"status"`
	Subject *string               `json:"subject,omitzero"`
}

// ChatDelivery is one reply or approval card a connection tried to post. Only failed and
// ambiguous deliveries need attention: the platform sends pending ones by itself.
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#delivery-outcomes.
type ChatDelivery struct {
	ID                ChatDeliveryID     `json:"id"`
	Kind              ChatDeliveryKind   `json:"kind"`
	Status            ChatDeliveryStatus `json:"status"`
	Attempt           int64              `json:"attempt"`
	CredentialVersion int64              `json:"credentialVersion"`
	Representation    string             `json:"representation"`
	Diagnostic        *string            `json:"diagnostic"`
	Receipts          json.RawMessage    `json:"receipts,omitzero"` // the platform's receipts, as sent
	SessionID         SessionID          `json:"sessionId"`
	MessageID         *string            `json:"messageId"`
	ApprovalID        *ApprovalID        `json:"approvalId"`
	ThreadID          string             `json:"threadId"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

// TenantChatDelivery is a delivery in the tenant-wide attention feed, with its connection.
type TenantChatDelivery struct {
	ChatDelivery
	ConnectionID ChatConnectionID `json:"connectionId"`
	AgentID      AgentID          `json:"agentId"`
	Platform     ChatPlatform     `json:"platform"`
}

// ChatDeliveryResult is a delivery after a repair. Check Status: a successful repair does not
// by itself mean the message was delivered.
type ChatDeliveryResult struct {
	ID           ChatDeliveryID     `json:"id"`
	SessionID    SessionID          `json:"sessionId"`
	ThreadID     string             `json:"threadId"`
	ConnectionID ChatConnectionID   `json:"connectionId"`
	Kind         ChatDeliveryKind   `json:"kind"`
	Status       ChatDeliveryStatus `json:"status"`
	Attempt      int64              `json:"attempt"`
}

// ChatConnectionSpec is the body of createChatConnection: SlackConnection or TelegramConnection.
type ChatConnectionSpec interface{ isChatConnectionSpec() }

// SlackConnection creates a Slack connection. The agent cannot be changed later.
type SlackConnection struct {
	AgentID     AgentID
	Name        string
	Enabled     *bool    // server default true
	ChannelIDs  []string // where health checks look; nil sends no configuration
	Credentials SlackCredentials
}

// TelegramConnection creates a Telegram connection. The agent cannot be changed later.
type TelegramConnection struct {
	AgentID      AgentID
	Name         string
	Enabled      *bool    // server default true
	ChatIDs      []string // where health checks look; nil omits it
	BusinessMode *bool    // server default false
	Credentials  TelegramCredentials
}

func (SlackConnection) isChatConnectionSpec()    {}
func (TelegramConnection) isChatConnectionSpec() {}

// ChatCredentials is the body of rotateChatCredentials: SlackCredentials or TelegramCredentials.
type ChatCredentials interface{ isChatCredentials() }

// SlackCredentials are a Slack bot's token and signing secret.
type SlackCredentials struct {
	BotToken      Secret
	SigningSecret Secret
}

// TelegramCredentials are a Telegram bot's token.
type TelegramCredentials struct {
	BotToken Secret
}

func (SlackCredentials) isChatCredentials()    {}
func (TelegramCredentials) isChatCredentials() {}

type connectionWire struct {
	AgentID       AgentID        `json:"agentId"`
	Name          string         `json:"name"`
	Enabled       *bool          `json:"enabled,omitzero"`
	Platform      string         `json:"platform"`
	Configuration map[string]any `json:"configuration,omitzero"`
	Credentials   map[string]any `json:"credentials"`
}

func (c SlackConnection) wire() (any, []string) {
	w := connectionWire{AgentID: c.AgentID, Name: c.Name, Enabled: c.Enabled, Platform: "slack"}
	if c.ChannelIDs != nil {
		w.Configuration = map[string]any{"channelIds": c.ChannelIDs}
	}
	w.Credentials, _ = c.Credentials.fields()
	return w, c.Credentials.secrets()
}

func (c TelegramConnection) wire() (any, []string) {
	w := connectionWire{AgentID: c.AgentID, Name: c.Name, Enabled: c.Enabled, Platform: "telegram"}
	if c.ChatIDs != nil || c.BusinessMode != nil {
		w.Configuration = map[string]any{}
		if c.ChatIDs != nil {
			w.Configuration["chatIds"] = c.ChatIDs
		}
		if c.BusinessMode != nil {
			w.Configuration["businessMode"] = *c.BusinessMode
		}
	}
	w.Credentials, _ = c.Credentials.fields()
	return w, c.Credentials.secrets()
}

// fields are the credentials' keys without platform, and the platform.
func (c SlackCredentials) fields() (map[string]any, string) {
	return map[string]any{"botToken": c.BotToken.Reveal(), "signingSecret": c.SigningSecret.Reveal()}, "slack"
}

func (c TelegramCredentials) fields() (map[string]any, string) {
	return map[string]any{"botToken": c.BotToken.Reveal()}, "telegram"
}

func (c SlackCredentials) secrets() []string {
	return []string{c.BotToken.Reveal(), c.SigningSecret.Reveal()}
}

func (c TelegramCredentials) secrets() []string { return []string{c.BotToken.Reveal()} }

func (c SlackCredentials) wire() (any, []string)    { return rotateWire(c.fields()), c.secrets() }
func (c TelegramCredentials) wire() (any, []string) { return rotateWire(c.fields()), c.secrets() }

// rotateWire is the flat rotateChatCredentials body: the only place credentials carry platform.
func rotateWire(fields map[string]any, platform string) map[string]any {
	fields["platform"] = platform
	return fields
}

// ChatConfigurationUpdate changes a connection's configuration. Slack accepts ChannelIDs, and
// Telegram accepts ChatIDs and BusinessMode. A nil list leaves it unchanged; pass []string{}
// to clear it.
type ChatConfigurationUpdate struct {
	BusinessMode *bool    `json:"businessMode,omitzero"`
	ChannelIDs   []string `json:"channelIds,omitzero"`
	ChatIDs      []string `json:"chatIds,omitzero"`
}

// UpdateChatConnectionParams is the body of renameChatConnection. Fields left nil keep their
// values.
type UpdateChatConnectionParams struct {
	Name          *string                  `json:"name,omitzero"`
	Configuration *ChatConfigurationUpdate `json:"configuration,omitzero"`
}

// ListChatDeliveriesParams pages listChatDeliveries.
type ListChatDeliveriesParams struct {
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// RepairChatDeliveryParams is the body of repairChatDelivery. The SDK always sends
// previousSenderStopped: true and acceptDuplicateRisk: true: the caller asserts that the
// previous sender stopped and accepts that the message may be delivered twice.
type RepairChatDeliveryParams struct {
	ExpectedAttempt int `json:"expectedAttempt"`
}

type repairBody struct {
	ExpectedAttempt       int  `json:"expectedAttempt"`
	PreviousSenderStopped bool `json:"previousSenderStopped"`
	AcceptDuplicateRisk   bool `json:"acceptDuplicateRisk"`
}

// ListTenantChatDeliveriesParams filters listTenantChatDeliveries. Keep the same filters
// across pages.
type ListTenantChatDeliveriesParams struct {
	Status []ChatDeliveryStatus `json:"-" query:"status"` // failed, ambiguous or both (the default); sent comma-joined
	Since  *time.Time           `json:"-" query:"since"`  // inclusive
	Cursor *string              `json:"-" query:"cursor"`
	Limit  *int                 `json:"-" query:"limit"`
}

var errNilParams = &ArgumentError{Name: "params", Reason: "nil"}

// List returns every chat connection of the tenant (listChatConnections).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#list-chat-connections.
func (s *ChatConnectionsService) List(ctx context.Context, opts ...RequestOption) ([]ChatConnection, error) {
	return list[ChatConnection]("chatConnections")(s.s.send(ctx, op.ListChatConnections(), nil, opts))
}

// Create connects an agent to Slack or Telegram (createChatConnection). params is a
// SlackConnection or a TelegramConnection, by value or pointer; nil is rejected.
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#create-chat-connection.
func (s *ChatConnectionsService) Create(ctx context.Context, params ChatConnectionSpec, opts ...RequestOption) (*ChatConnection, error) {
	if params == nil { // a typed nil is rejected by the encoder
		return nil, errNilParams
	}
	return decode[ChatConnection](s.s.send(ctx, op.CreateChatConnection(), params, opts))
}

// Get returns a chat connection (getChatConnection).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#get-chat-connection.
func (s *ChatConnectionsService) Get(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.GetChatConnection(string(connectionID)), nil, opts))
}

// Update renames a connection or changes its configuration (renameChatConnection). Changing
// BusinessMode on an enabled Telegram connection re-registers its webhook; if that fails the
// change is still saved, the call returns an error, and Enable retries it.
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#rename-chat-connection.
func (s *ChatConnectionsService) Update(ctx context.Context, connectionID ChatConnectionID, params UpdateChatConnectionParams, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.RenameChatConnection(string(connectionID)), params, opts))
}

// Delete deletes a chat connection (deleteChatConnection).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#delete-chat-connection.
func (s *ChatConnectionsService) Delete(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteChatConnection(string(connectionID)), nil, opts))
}

// Enable lets a connection accept messages again (enableChatConnection).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#enable-chat-connection.
func (s *ChatConnectionsService) Enable(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.EnableChatConnection(string(connectionID)), nil, opts))
}

// Disable stops a connection from accepting messages (disableChatConnection).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#disable-chat-connection.
func (s *ChatConnectionsService) Disable(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.DisableChatConnection(string(connectionID)), nil, opts))
}

// CheckHealth runs a connection's health checks now and returns the connection with the
// result in Health (checkChatHealth).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#check-chat-health.
func (s *ChatConnectionsService) CheckHealth(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.CheckChatHealth(string(connectionID)), nil, opts))
}

// RotateCredentials replaces a connection's bot credentials (rotateChatCredentials). params is
// SlackCredentials or TelegramCredentials, by value or pointer, and must match the
// connection's platform; nil is rejected.
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#rotate-chat-credentials.
func (s *ChatConnectionsService) RotateCredentials(ctx context.Context, connectionID ChatConnectionID, params ChatCredentials, opts ...RequestOption) (*ChatConnection, error) {
	if params == nil { // a typed nil is rejected by the encoder
		return nil, errNilParams
	}
	return decode[ChatConnection](s.s.send(ctx, op.RotateChatCredentials(string(connectionID)), params, opts))
}

// ListDeliveries returns one page of a connection's deliveries, newest first, in every status
// (listChatDeliveries).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#list-chat-deliveries.
func (s *ChatConnectionsService) ListDeliveries(ctx context.Context, connectionID ChatConnectionID, params ListChatDeliveriesParams, opts ...RequestOption) (*Page[ChatDelivery], error) {
	return decode[Page[ChatDelivery]](s.s.send(ctx, op.ListChatDeliveries(string(connectionID)), params, opts))
}

// AllDeliveries iterates over a connection's deliveries from params.Cursor on, fetching pages
// as needed (listChatDeliveries).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#list-chat-deliveries.
func (s *ChatConnectionsService) AllDeliveries(ctx context.Context, connectionID ChatConnectionID, params ListChatDeliveriesParams, opts ...RequestOption) iter.Seq2[ChatDelivery, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[ChatDelivery], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListDeliveries(ctx, connectionID, p, opts...)
	})
}

// RepairDelivery sends a stuck reply or approval card again, without running a turn
// (repairChatDelivery). ExpectedAttempt is the delivery's current Attempt (0 is valid); the
// repair is refused if it changed. The SDK always sends previousSenderStopped: true and
// acceptDuplicateRisk: true, so calling it asserts that the previous sender has stopped and
// accepts that the message may appear twice. Check the result's Status.
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#repair-chat-delivery.
func (s *ChatConnectionsService) RepairDelivery(ctx context.Context, connectionID ChatConnectionID, deliveryID ChatDeliveryID, params RepairChatDeliveryParams, opts ...RequestOption) (*ChatDeliveryResult, error) {
	return decode[ChatDeliveryResult](s.s.send(ctx, op.RepairChatDelivery(string(connectionID), string(deliveryID)), repairBody{params.ExpectedAttempt, true, true}, opts))
}

// List returns one page of the tenant's failed and ambiguous deliveries across every
// connection, newest first (listTenantChatDeliveries).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#list-tenant-chat-deliveries.
func (s *ChatDeliveriesService) List(ctx context.Context, params ListTenantChatDeliveriesParams, opts ...RequestOption) (*Page[TenantChatDelivery], error) {
	return decode[Page[TenantChatDelivery]](s.s.send(ctx, op.ListTenantChatDeliveries(), params, opts))
}

// All iterates over the attention feed from params.Cursor on, fetching pages as needed
// (listTenantChatDeliveries).
// See https://docs.blazingagents.com/api-reference/rest-api/chat-connections#list-tenant-chat-deliveries.
func (s *ChatDeliveriesService) All(ctx context.Context, params ListTenantChatDeliveriesParams, opts ...RequestOption) iter.Seq2[TenantChatDelivery, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*Page[TenantChatDelivery], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}
