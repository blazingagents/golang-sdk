package blazingagents

import (
	"context"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ProviderSummary is a provider in a list.
type ProviderSummary struct {
	ID ProviderID `json:"id"`
}

// Provider is a model provider and its key's fragment.
type Provider struct {
	ID   ProviderID `json:"id"`
	Name string     `json:"name"`
}

// ProviderModel is a model a provider offers.
type ProviderModel struct {
	ID string `json:"id"`
}

// ProviderThinkingLevels are the thinking levels a model accepts.
type ProviderThinkingLevels struct {
	Known bool `json:"known"`
}

// CreateProviderParams is the body of createProvider.
type CreateProviderParams struct {
	Name         string           `json:"name"`
	ProviderType ProviderType     `json:"providerType"`
	BaseURL      Nullable[string] `json:"baseUrl,omitzero"`
	APIKey       Secret           `json:"apiKey"`
}

type createProviderWire struct {
	Name         string           `json:"name"`
	ProviderType ProviderType     `json:"providerType"`
	BaseURL      Nullable[string] `json:"baseUrl,omitzero"`
	APIKey       string           `json:"apiKey"`
}

func (p CreateProviderParams) wire() (any, []string) {
	key := p.APIKey.Reveal()
	return createProviderWire{p.Name, p.ProviderType, p.BaseURL, key}, []string{key}
}

// UpdateProviderParams is the body of updateProvider.
type UpdateProviderParams struct {
	Name string `json:"name"`
}

// DeleteProviderParams is the query of deleteProvider.
type DeleteProviderParams struct {
	ConfirmSnapshotInvalidation *bool `json:"-" query:"confirmSnapshotInvalidation"`
}

type thinkingLevelsQuery struct {
	Model string `query:"model"`
}

// List is listProviders.
func (s *ProvidersService) List(ctx context.Context, opts ...RequestOption) ([]ProviderSummary, error) {
	return list[ProviderSummary]("providers")(s.s.send(ctx, op.ListProviders(), nil, opts))
}

// Create is createProvider.
func (s *ProvidersService) Create(ctx context.Context, params CreateProviderParams, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.CreateProvider(), params, opts))
}

// Get is getProvider.
func (s *ProvidersService) Get(ctx context.Context, providerID ProviderID, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.GetProvider(string(providerID)), nil, opts))
}

// Update is updateProvider.
func (s *ProvidersService) Update(ctx context.Context, providerID ProviderID, params UpdateProviderParams, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.UpdateProvider(string(providerID)), params, opts))
}

// Delete is deleteProvider.
func (s *ProvidersService) Delete(ctx context.Context, providerID ProviderID, params DeleteProviderParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteProvider(string(providerID)), params, opts))
}

// ListModels is listProviderModels.
func (s *ProvidersService) ListModels(ctx context.Context, providerID ProviderID, opts ...RequestOption) ([]ProviderModel, error) {
	return list[ProviderModel]("models")(s.s.send(ctx, op.ListProviderModels(string(providerID)), nil, opts))
}

// ThinkingLevels is listThinkingLevels.
func (s *ProvidersService) ThinkingLevels(ctx context.Context, providerID ProviderID, model string, opts ...RequestOption) (*ProviderThinkingLevels, error) {
	return decode[ProviderThinkingLevels](s.s.send(ctx, op.ListThinkingLevels(string(providerID)), thinkingLevelsQuery{model}, opts))
}
