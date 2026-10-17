package blazingagents

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// MerchantConnection is the tenant's connection to its Polar or Dodo account. The credential
// is never returned; KeyFragment holds its last few characters.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant.
type MerchantConnection struct {
	ID                MerchantConnectionID     `json:"id"`
	Provider          MerchantProvider         `json:"provider"`
	Environment       MerchantEnvironment      `json:"environment"`
	Status            MerchantConnectionStatus `json:"status"`
	MerchantAccountID string                   `json:"merchantAccountId"`
	KeyFragment       string                   `json:"keyFragment"`
	Guard             MerchantGuard            `json:"guard"`
	ConfigVersion     int64                    `json:"configVersion"`
	CreatedAt         time.Time                `json:"createdAt"`
	UpdatedAt         time.Time                `json:"updatedAt"`
}

// MerchantGuard checks each end user's plan or balance with the provider before every turn
// starts. An enabled guard needs at least one product ID or a meter ID.
type MerchantGuard struct {
	Enabled    bool     `json:"enabled"`
	ProductIDs []string `json:"productIds"` // nil sends []
	MeterID    *string  `json:"meterId"`    // nil sends null
}

// MarshalJSON sends a nil ProductIDs as [], since the field is required.
func (g MerchantGuard) MarshalJSON() ([]byte, error) {
	type plain MerchantGuard
	if g.ProductIDs == nil {
		g.ProductIDs = []string{}
	}
	return json.Marshal(plain(g))
}

// CreateMerchantConnectionParams is the body of createMerchantConnection. Credential is a
// Polar Organization Access Token, or a Dodo API key in the mode that matches Environment.
type CreateMerchantConnectionParams struct {
	Provider    MerchantProvider    `json:"provider"`
	Environment MerchantEnvironment `json:"environment"`
	Credential  Secret              `json:"credential"`
	Guard       *MerchantGuard      `json:"guard,omitzero"` // nil leaves the guard off
}

type createMerchantWire struct {
	Provider    MerchantProvider    `json:"provider"`
	Environment MerchantEnvironment `json:"environment"`
	Credential  string              `json:"credential"`
	Guard       *MerchantGuard      `json:"guard,omitzero"`
}

func (p CreateMerchantConnectionParams) wire() (any, []string) {
	c := p.Credential.Reveal()
	return createMerchantWire{p.Provider, p.Environment, c, p.Guard}, []string{c}
}

// UpdateMerchantConnectionParams is the body of updateMerchantConnection. Fields left zero
// keep their values.
type UpdateMerchantConnectionParams struct {
	Credential Secret                    `json:"credential,omitzero"` // same merchant account; the zero Secret leaves it unchanged
	Status     *MerchantConnectionStatus `json:"status,omitzero"`
	Guard      *MerchantGuard            `json:"guard,omitzero"` // replaces the whole guard
}

type updateMerchantWire struct {
	Credential *string                   `json:"credential,omitzero"`
	Status     *MerchantConnectionStatus `json:"status,omitzero"`
	Guard      *MerchantGuard            `json:"guard,omitzero"`
}

func (p UpdateMerchantConnectionParams) wire() (any, []string) {
	w := updateMerchantWire{Status: p.Status, Guard: p.Guard}
	if p.Credential.IsZero() {
		return w, nil
	}
	c := p.Credential.Reveal()
	w.Credential = &c
	return w, []string{c}
}

// MerchantBinding links an end user to a customer in the billing provider. Bindings belong to
// a connection: link users again after replacing it.
type MerchantBinding struct {
	UserID     string    `json:"userId"`
	CustomerID string    `json:"customerId"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// MerchantBindingList is one page of bindings.
type MerchantBindingList struct {
	Bindings   []MerchantBinding `json:"bindings"`
	NextCursor string            `json:"nextCursor"` // "" on the last page
}

func (l *MerchantBindingList) items() []MerchantBinding { return l.Bindings }
func (l *MerchantBindingList) next() string             { return l.NextCursor }

// ListMerchantBindingsParams filters listMerchantBindings.
type ListMerchantBindingsParams struct {
	UserID *string `json:"-" query:"userId"` // only this end user's binding
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// SetMerchantBindingParams is the body of setMerchantBinding.
type SetMerchantBindingParams struct {
	CustomerID string `json:"customerId"`
}

// MerchantUsageEvent is the model-token usage of one turn, sent to the billing provider.
// NextAction says what, if anything, to do about it.
type MerchantUsageEvent struct {
	ID                MerchantUsageEventID     `json:"id"`
	TurnID            TurnID                   `json:"turnId"`
	ConnectionID      MerchantConnectionID     `json:"connectionId"`
	Provider          MerchantProvider         `json:"provider"`
	UserID            string                   `json:"userId"`
	CustomerID        *string                  `json:"customerId"` // nil until the user is bound
	AgentID           AgentID                  `json:"agentId"`
	SessionID         *SessionID               `json:"sessionId"`
	Model             string                   `json:"model"`
	ModelProvider     ProviderType             `json:"modelProvider"`
	InputTokens       int64                    `json:"inputTokens"`
	OutputTokens      int64                    `json:"outputTokens"`
	TotalTokens       int64                    `json:"totalTokens"`
	TurnStatus        MerchantTurnStatus       `json:"turnStatus"`
	StartedAt         time.Time                `json:"startedAt"`
	OccurredAt        time.Time                `json:"occurredAt"`
	Status            MerchantUsageEventStatus `json:"status"`
	Payload           Metadata                 `json:"payload"` // what the provider is sent; nil until known
	AttemptCount      int64                    `json:"attemptCount"`
	AttemptGeneration int64                    `json:"attemptGeneration"`
	LastAttemptAt     *time.Time               `json:"lastAttemptAt"`
	LastErrorCode     *string                  `json:"lastErrorCode"`
	AcceptedAt        *time.Time               `json:"acceptedAt"`
	WorkflowIssue     *MerchantWorkflowIssue   `json:"workflowIssue"`
	NextAction        MerchantUsageEventAction `json:"nextAction"`
}

// MerchantUsageEventList is one page of usage events.
type MerchantUsageEventList struct {
	Events     []MerchantUsageEvent `json:"events"`
	NextCursor string               `json:"nextCursor"` // "" on the last page
}

func (l *MerchantUsageEventList) items() []MerchantUsageEvent { return l.Events }
func (l *MerchantUsageEventList) next() string                { return l.NextCursor }

// ListMerchantUsageEventsParams filters listMerchantUsageEvents.
type ListMerchantUsageEventsParams struct {
	Status *MerchantUsageEventStatus `json:"-" query:"status"`
	Cursor *string                   `json:"-" query:"cursor"`
	Limit  *int                      `json:"-" query:"limit"`
}

// MerchantUsageSummary counts usage events by status and totals accepted usage per UTC day.
type MerchantUsageSummary struct {
	Counts                  MerchantUsageCounts `json:"counts"`
	LastAcceptedAt          *time.Time          `json:"lastAcceptedAt"`
	OldestPendingOccurredAt *time.Time          `json:"oldestPendingOccurredAt"`
	Daily                   []MerchantUsageDay  `json:"daily"`
}

// MerchantUsageCounts is the number of usage events in each status.
type MerchantUsageCounts struct {
	Pending    int64 `json:"pending"`
	Accepted   int64 `json:"accepted"`
	Uncertain  int64 `json:"uncertain"`
	Failed     int64 `json:"failed"`
	Unmapped   int64 `json:"unmapped"`
	Incomplete int64 `json:"incomplete"`
	Expired    int64 `json:"expired"`
	Discarded  int64 `json:"discarded"`
}

// MerchantUsageDay is one UTC day of the summary.
type MerchantUsageDay struct {
	Day            Date  `json:"day"`
	AcceptedEvents int64 `json:"acceptedEvents"`
	AcceptedTokens int64 `json:"acceptedTokens"`
	PendingEvents  int64 `json:"pendingEvents"`
}

// MerchantUsageSummaryParams is the query of getMerchantUsageSummary.
type MerchantUsageSummaryParams struct {
	Days *int `json:"-" query:"days"` // days in Daily, ending today; server default 14
}

// Get returns the tenant's merchant connection (getMerchantConnection). It returns nil, nil
// when the tenant has none: the one successful call that returns no model.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#get-merchant-connection.
func (s *MerchantConnectionService) Get(ctx context.Context, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrapOptional[MerchantConnection]("connection")(s.s.send(ctx, op.GetMerchantConnection(), nil, opts))
}

// Create connects the tenant's Polar or Dodo account, replacing the current connection
// (createMerchantConnection). The provider checks the credential first. Monetization stays off
// until the tenant enables it.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#create-merchant-connection.
func (s *MerchantConnectionService) Create(ctx context.Context, params CreateMerchantConnectionParams, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrap[MerchantConnection]("connection")(s.s.send(ctx, op.CreateMerchantConnection(), params, opts))
}

// Update changes the connection's status or guard, or rotates its credential
// (updateMerchantConnection).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#update-merchant-connection.
func (s *MerchantConnectionService) Update(ctx context.Context, params UpdateMerchantConnectionParams, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrap[MerchantConnection]("connection")(s.s.send(ctx, op.UpdateMerchantConnection(), params, opts))
}

// Retire retires the connection (retireMerchantConnection). Events already recorded keep being
// delivered with its credential.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#retire-merchant-connection.
func (s *MerchantConnectionService) Retire(ctx context.Context, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.RetireMerchantConnection(), nil, opts))
}

// List returns one page of the connection's customer bindings (listMerchantBindings).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#list-merchant-bindings.
func (s *MerchantBindingsService) List(ctx context.Context, params ListMerchantBindingsParams, opts ...RequestOption) (*MerchantBindingList, error) {
	return decode[MerchantBindingList](s.s.send(ctx, op.ListMerchantBindings(), params, opts))
}

// All iterates over the bindings from params.Cursor on, fetching pages as needed
// (listMerchantBindings).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#list-merchant-bindings.
func (s *MerchantBindingsService) All(ctx context.Context, params ListMerchantBindingsParams, opts ...RequestOption) iter.Seq2[MerchantBinding, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*MerchantBindingList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Set links an end user to a customer in the billing provider, replacing any earlier link
// (setMerchantBinding). userID is the ID the tenant sends with turns.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#set-merchant-binding.
func (s *MerchantBindingsService) Set(ctx context.Context, userID string, params SetMerchantBindingParams, opts ...RequestOption) (*MerchantBinding, error) {
	return unwrap[MerchantBinding]("binding")(s.s.send(ctx, op.SetMerchantBinding(userID), params, opts))
}

// Delete removes an end user's binding (deleteMerchantBinding).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#delete-merchant-binding.
func (s *MerchantBindingsService) Delete(ctx context.Context, userID string, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMerchantBinding(userID), nil, opts))
}

// List returns one page of usage events, newest first (listMerchantUsageEvents).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#list-merchant-usage-events.
func (s *MerchantUsageEventsService) List(ctx context.Context, params ListMerchantUsageEventsParams, opts ...RequestOption) (*MerchantUsageEventList, error) {
	return decode[MerchantUsageEventList](s.s.send(ctx, op.ListMerchantUsageEvents(), params, opts))
}

// All iterates over the usage events from params.Cursor on, fetching pages as needed
// (listMerchantUsageEvents).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#list-merchant-usage-events.
func (s *MerchantUsageEventsService) All(ctx context.Context, params ListMerchantUsageEventsParams, opts ...RequestOption) iter.Seq2[MerchantUsageEvent, error] {
	return all(ctx, func(ctx context.Context, cursor string) (*MerchantUsageEventList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Summary returns the usage event counts and daily totals (getMerchantUsageSummary).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#get-merchant-usage-summary.
func (s *MerchantUsageEventsService) Summary(ctx context.Context, params MerchantUsageSummaryParams, opts ...RequestOption) (*MerchantUsageSummary, error) {
	return unwrap[MerchantUsageSummary]("summary")(s.s.send(ctx, op.GetMerchantUsageSummary(), params, opts))
}

// Get returns a usage event (getMerchantUsageEvent).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#get-merchant-usage-event.
func (s *MerchantUsageEventsService) Get(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.GetMerchantUsageEvent(string(eventID)), nil, opts))
}

// Retry sends a failed or uncertain event, or a pending one automatic delivery gave up on, to
// the provider again (retryMerchantUsageEvent). The provider drops duplicates by event ID.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#retry-merchant-usage-event.
func (s *MerchantUsageEventsService) Retry(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.RetryMerchantUsageEvent(string(eventID)), nil, opts))
}

// Release sends an unmapped event after its user is bound to a customer
// (releaseMerchantUsageEvent). Bind the user with MerchantBindings.Set first.
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#release-merchant-usage-event.
func (s *MerchantUsageEventsService) Release(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.ReleaseMerchantUsageEvent(string(eventID)), nil, opts))
}

// Discard stops delivery of an event and marks it discarded (discardMerchantUsageEvent).
// See https://docs.blazingagents.com/api-reference/rest-api/merchant#discard-merchant-usage-event.
func (s *MerchantUsageEventsService) Discard(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.DiscardMerchantUsageEvent(string(eventID)), nil, opts))
}
