package blazingagents

import (
	"context"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ChatConnection is an agent's connection to a Slack or Telegram bot.
type ChatConnection struct {
	ID            ChatConnectionID  `json:"id"`
	Configuration ChatConfiguration `json:"configuration"`
}

// ChatDelivery is one delivery of a connection.
type ChatDelivery struct {
	ID ChatDeliveryID `json:"id"`
}

// TenantChatDelivery is a delivery across the tenant's connections.
type TenantChatDelivery struct {
	ID ChatDeliveryID `json:"id"`
}

// ChatDeliveryResult is a repaired delivery.
type ChatDeliveryResult struct {
	ID ChatDeliveryID `json:"id"`
}

// ChatConnectionSpec is the body of createChatConnection: SlackConnection or TelegramConnection.
type ChatConnectionSpec interface{ isChatConnectionSpec() }

// SlackConnection creates a Slack connection.
type SlackConnection struct {
	AgentID     AgentID
	Name        string
	Enabled     *bool
	ChannelIDs  []string // nil sends no configuration
	Credentials SlackCredentials
}

// TelegramConnection creates a Telegram connection.
type TelegramConnection struct {
	AgentID      AgentID
	Name         string
	Enabled      *bool
	ChatIDs      []string
	BusinessMode *bool
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

// ChatConfigurationUpdate changes a connection's channels or chats.
type ChatConfigurationUpdate struct {
	BusinessMode *bool    `json:"businessMode,omitzero"`
	ChannelIDs   []string `json:"channelIds,omitzero"`
	ChatIDs      []string `json:"chatIds,omitzero"`
}

// UpdateChatConnectionParams is the body of renameChatConnection.
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

// ListTenantChatDeliveriesParams filters listTenantChatDeliveries.
type ListTenantChatDeliveriesParams struct {
	Status []ChatDeliveryStatus `json:"-" query:"status"` // comma-joined
	Since  *time.Time           `json:"-" query:"since"`
	Cursor *string              `json:"-" query:"cursor"`
	Limit  *int                 `json:"-" query:"limit"`
}

var errNilParams = &ArgumentError{Name: "params", Reason: "nil"}

// List is listChatConnections.
func (s *ChatConnectionsService) List(ctx context.Context, opts ...RequestOption) ([]ChatConnection, error) {
	return list[ChatConnection]("chatConnections")(s.s.send(ctx, op.ListChatConnections(), nil, opts))
}

// Create is createChatConnection.
func (s *ChatConnectionsService) Create(ctx context.Context, params ChatConnectionSpec, opts ...RequestOption) (*ChatConnection, error) {
	if params == nil {
		return nil, errNilParams
	}
	return decode[ChatConnection](s.s.send(ctx, op.CreateChatConnection(), params, opts))
}

// Get is getChatConnection.
func (s *ChatConnectionsService) Get(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.GetChatConnection(string(connectionID)), nil, opts))
}

// Update is renameChatConnection.
func (s *ChatConnectionsService) Update(ctx context.Context, connectionID ChatConnectionID, params UpdateChatConnectionParams, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.RenameChatConnection(string(connectionID)), params, opts))
}

// Delete is deleteChatConnection.
func (s *ChatConnectionsService) Delete(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteChatConnection(string(connectionID)), nil, opts))
}

// Enable is enableChatConnection.
func (s *ChatConnectionsService) Enable(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.EnableChatConnection(string(connectionID)), nil, opts))
}

// Disable is disableChatConnection.
func (s *ChatConnectionsService) Disable(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.DisableChatConnection(string(connectionID)), nil, opts))
}

// CheckHealth is checkChatHealth.
func (s *ChatConnectionsService) CheckHealth(ctx context.Context, connectionID ChatConnectionID, opts ...RequestOption) (*ChatConnection, error) {
	return decode[ChatConnection](s.s.send(ctx, op.CheckChatHealth(string(connectionID)), nil, opts))
}

// RotateCredentials is rotateChatCredentials.
func (s *ChatConnectionsService) RotateCredentials(ctx context.Context, connectionID ChatConnectionID, params ChatCredentials, opts ...RequestOption) (*ChatConnection, error) {
	if params == nil {
		return nil, errNilParams
	}
	return decode[ChatConnection](s.s.send(ctx, op.RotateChatCredentials(string(connectionID)), params, opts))
}

// ListDeliveries is listChatDeliveries.
func (s *ChatConnectionsService) ListDeliveries(ctx context.Context, connectionID ChatConnectionID, params ListChatDeliveriesParams, opts ...RequestOption) (*Page[ChatDelivery], error) {
	return decode[Page[ChatDelivery]](s.s.send(ctx, op.ListChatDeliveries(string(connectionID)), params, opts))
}

// AllDeliveries iterates listChatDeliveries across pages.
func (s *ChatConnectionsService) AllDeliveries(ctx context.Context, connectionID ChatConnectionID, params ListChatDeliveriesParams, opts ...RequestOption) iter.Seq2[ChatDelivery, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[ChatDelivery], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.ListDeliveries(ctx, connectionID, p, opts...)
	})
}

// RepairDelivery is repairChatDelivery. ExpectedAttempt 0 is valid.
func (s *ChatConnectionsService) RepairDelivery(ctx context.Context, connectionID ChatConnectionID, deliveryID ChatDeliveryID, params RepairChatDeliveryParams, opts ...RequestOption) (*ChatDeliveryResult, error) {
	return decode[ChatDeliveryResult](s.s.send(ctx, op.RepairChatDelivery(string(connectionID), string(deliveryID)), repairBody{params.ExpectedAttempt, true, true}, opts))
}

// List is listTenantChatDeliveries.
func (s *ChatDeliveriesService) List(ctx context.Context, params ListTenantChatDeliveriesParams, opts ...RequestOption) (*Page[TenantChatDelivery], error) {
	return decode[Page[TenantChatDelivery]](s.s.send(ctx, op.ListTenantChatDeliveries(), params, opts))
}

// All iterates listTenantChatDeliveries across pages.
func (s *ChatDeliveriesService) All(ctx context.Context, params ListTenantChatDeliveriesParams, opts ...RequestOption) iter.Seq2[TenantChatDelivery, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*Page[TenantChatDelivery], error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}
