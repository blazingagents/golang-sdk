package blazingagents

import (
	"context"
	"iter"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// MerchantConnection is the tenant's billing provider connection.
type MerchantConnection struct {
	ID MerchantConnectionID `json:"id"`
}

// MerchantGuard gates usage on an active subscription.
type MerchantGuard struct {
	Enabled    bool     `json:"enabled"`
	ProductIDs []string `json:"productIds"`
	MeterID    *string  `json:"meterId"` // nil sends null
}

// CreateMerchantConnectionParams is the body of createMerchantConnection.
type CreateMerchantConnectionParams struct {
	Provider    MerchantProvider    `json:"provider"`
	Environment MerchantEnvironment `json:"environment"`
	Credential  Secret              `json:"credential"`
	Guard       *MerchantGuard      `json:"guard,omitzero"`
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

// UpdateMerchantConnectionParams is the body of updateMerchantConnection.
type UpdateMerchantConnectionParams struct {
	Credential Secret                    `json:"credential,omitzero"` // the zero Secret leaves it unchanged
	Status     *MerchantConnectionStatus `json:"status,omitzero"`
	Guard      *MerchantGuard            `json:"guard,omitzero"`
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

// MerchantBinding maps an end user to a billing customer.
type MerchantBinding struct {
	UserID string `json:"userId"`
}

// MerchantBindingList is one page of bindings.
type MerchantBindingList struct {
	Bindings   []MerchantBinding `json:"bindings"`
	NextCursor string            `json:"nextCursor"`
}

func (l *MerchantBindingList) items() []MerchantBinding { return l.Bindings }
func (l *MerchantBindingList) next() string             { return l.NextCursor }

// ListMerchantBindingsParams filters listMerchantBindings.
type ListMerchantBindingsParams struct {
	UserID *string `json:"-" query:"userId"`
	Cursor *string `json:"-" query:"cursor"`
	Limit  *int    `json:"-" query:"limit"`
}

// SetMerchantBindingParams is the body of setMerchantBinding.
type SetMerchantBindingParams struct {
	CustomerID string `json:"customerId"`
}

// MerchantUsageEvent is usage reported to the billing provider.
type MerchantUsageEvent struct {
	ID MerchantUsageEventID `json:"id"`
}

// MerchantUsageEventList is one page of usage events.
type MerchantUsageEventList struct {
	Events     []MerchantUsageEvent `json:"events"`
	NextCursor string               `json:"nextCursor"`
}

func (l *MerchantUsageEventList) items() []MerchantUsageEvent { return l.Events }
func (l *MerchantUsageEventList) next() string                { return l.NextCursor }

// ListMerchantUsageEventsParams filters listMerchantUsageEvents.
type ListMerchantUsageEventsParams struct {
	Status *MerchantUsageEventStatus `json:"-" query:"status"`
	Cursor *string                   `json:"-" query:"cursor"`
	Limit  *int                      `json:"-" query:"limit"`
}

// MerchantUsageSummary counts usage events by state.
type MerchantUsageSummary struct {
	Counts map[MerchantUsageEventStatus]int64 `json:"counts"`
}

// MerchantUsageSummaryParams is the query of getMerchantUsageSummary.
type MerchantUsageSummaryParams struct {
	Days *int `json:"-" query:"days"`
}

// Get is getMerchantConnection. It returns nil, nil when the tenant has no connection.
func (s *MerchantConnectionService) Get(ctx context.Context, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrapOptional[MerchantConnection]("connection")(s.s.send(ctx, op.GetMerchantConnection(), nil, opts))
}

// Create is createMerchantConnection.
func (s *MerchantConnectionService) Create(ctx context.Context, params CreateMerchantConnectionParams, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrap[MerchantConnection]("connection")(s.s.send(ctx, op.CreateMerchantConnection(), params, opts))
}

// Update is updateMerchantConnection.
func (s *MerchantConnectionService) Update(ctx context.Context, params UpdateMerchantConnectionParams, opts ...RequestOption) (*MerchantConnection, error) {
	return unwrap[MerchantConnection]("connection")(s.s.send(ctx, op.UpdateMerchantConnection(), params, opts))
}

// Retire is retireMerchantConnection.
func (s *MerchantConnectionService) Retire(ctx context.Context, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.RetireMerchantConnection(), nil, opts))
}

// List is listMerchantBindings.
func (s *MerchantBindingsService) List(ctx context.Context, params ListMerchantBindingsParams, opts ...RequestOption) (*MerchantBindingList, error) {
	return decode[MerchantBindingList](s.s.send(ctx, op.ListMerchantBindings(), params, opts))
}

// All iterates listMerchantBindings across pages.
func (s *MerchantBindingsService) All(ctx context.Context, params ListMerchantBindingsParams, opts ...RequestOption) iter.Seq2[MerchantBinding, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*MerchantBindingList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Set is setMerchantBinding.
func (s *MerchantBindingsService) Set(ctx context.Context, userID string, params SetMerchantBindingParams, opts ...RequestOption) (*MerchantBinding, error) {
	return unwrap[MerchantBinding]("binding")(s.s.send(ctx, op.SetMerchantBinding(userID), params, opts))
}

// Delete is deleteMerchantBinding.
func (s *MerchantBindingsService) Delete(ctx context.Context, userID string, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteMerchantBinding(userID), nil, opts))
}

// List is listMerchantUsageEvents.
func (s *MerchantUsageEventsService) List(ctx context.Context, params ListMerchantUsageEventsParams, opts ...RequestOption) (*MerchantUsageEventList, error) {
	return decode[MerchantUsageEventList](s.s.send(ctx, op.ListMerchantUsageEvents(), params, opts))
}

// All iterates listMerchantUsageEvents across pages.
func (s *MerchantUsageEventsService) All(ctx context.Context, params ListMerchantUsageEventsParams, opts ...RequestOption) iter.Seq2[MerchantUsageEvent, error] {
	return all(ctx, "", func(ctx context.Context, cursor string) (*MerchantUsageEventList, error) {
		p := params
		if cursor != "" {
			p.Cursor = &cursor
		}
		return s.List(ctx, p, opts...)
	})
}

// Summary is getMerchantUsageSummary.
func (s *MerchantUsageEventsService) Summary(ctx context.Context, params MerchantUsageSummaryParams, opts ...RequestOption) (*MerchantUsageSummary, error) {
	return unwrap[MerchantUsageSummary]("summary")(s.s.send(ctx, op.GetMerchantUsageSummary(), params, opts))
}

// Get is getMerchantUsageEvent.
func (s *MerchantUsageEventsService) Get(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.GetMerchantUsageEvent(string(eventID)), nil, opts))
}

// Retry is retryMerchantUsageEvent.
func (s *MerchantUsageEventsService) Retry(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.RetryMerchantUsageEvent(string(eventID)), nil, opts))
}

// Release is releaseMerchantUsageEvent.
func (s *MerchantUsageEventsService) Release(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.ReleaseMerchantUsageEvent(string(eventID)), nil, opts))
}

// Discard is discardMerchantUsageEvent.
func (s *MerchantUsageEventsService) Discard(ctx context.Context, eventID MerchantUsageEventID, opts ...RequestOption) (*MerchantUsageEvent, error) {
	return unwrap[MerchantUsageEvent]("event")(s.s.send(ctx, op.DiscardMerchantUsageEvent(string(eventID)), nil, opts))
}
