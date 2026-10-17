package blazingagents

import (
	"context"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

// ProviderSummary is a provider in the list of listProviders.
type ProviderSummary struct {
	ID           ProviderID   `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"providerType"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// Provider is a model provider. The API never returns its key, only the key's last
// characters in KeyFragment.
type Provider struct {
	ID           ProviderID   `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"providerType"`
	BaseURL      *string      `json:"baseUrl"`
	KeyFragment  string       `json:"keyFragment"`
	CreatedAt    time.Time    `json:"createdAt"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

// ProviderModel is a model a provider offers.
type ProviderModel struct {
	ID string `json:"id"`
}

// ProviderThinkingLevels are the thinking levels a model accepts. Known is false, with no
// levels, when the model's capabilities could not be looked up. A known empty list means
// only the provider's default is available.
type ProviderThinkingLevels struct {
	Known  bool     `json:"known"`
	Levels []string `json:"levels"`
}

// CreateProviderParams is the body of createProvider. BaseURL overrides the endpoint: it is
// required for ProviderTypeCustom and not accepted for ProviderTypeVercelAIGateway. Omitted
// and Null both leave it unset. None of the fields but Name can change later.
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

// DeleteProviderParams is the query of deleteProvider. ConfirmSnapshotInvalidation set to
// true deletes a provider that saved session or task run configurations still reference
// (error code provider_historical_use otherwise). It never overrides use by a current agent.
type DeleteProviderParams struct {
	ConfirmSnapshotInvalidation *bool `json:"-" query:"confirmSnapshotInvalidation"`
}

type thinkingLevelsQuery struct {
	Model string `query:"model"`
}

// List lists the tenant's providers. It sends listProviders:
// https://docs.blazingagents.com/api-reference/rest-api/providers#list-providers
func (s *ProvidersService) List(ctx context.Context, opts ...RequestOption) ([]ProviderSummary, error) {
	return list[ProviderSummary]("providers")(s.s.send(ctx, op.ListProviders(), nil, opts))
}

// Create adds a provider. It sends createProvider:
// https://docs.blazingagents.com/api-reference/rest-api/providers#create-provider
func (s *ProvidersService) Create(ctx context.Context, params CreateProviderParams, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.CreateProvider(), params, opts))
}

// Get returns a provider. It sends getProvider:
// https://docs.blazingagents.com/api-reference/rest-api/providers#get-provider
func (s *ProvidersService) Get(ctx context.Context, providerID ProviderID, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.GetProvider(string(providerID)), nil, opts))
}

// Update renames a provider. It sends updateProvider:
// https://docs.blazingagents.com/api-reference/rest-api/providers#update-provider
func (s *ProvidersService) Update(ctx context.Context, providerID ProviderID, params UpdateProviderParams, opts ...RequestOption) (*Provider, error) {
	return decode[Provider](s.s.send(ctx, op.UpdateProvider(string(providerID)), params, opts))
}

// Delete deletes a provider and its key. It sends deleteProvider:
// https://docs.blazingagents.com/api-reference/rest-api/providers#delete-provider
func (s *ProvidersService) Delete(ctx context.Context, providerID ProviderID, params DeleteProviderParams, opts ...RequestOption) error {
	return empty(s.s.send(ctx, op.DeleteProvider(string(providerID)), params, opts))
}

// ListModels lists the models a provider offers. It sends listProviderModels:
// https://docs.blazingagents.com/api-reference/rest-api/providers#list-provider-models
func (s *ProvidersService) ListModels(ctx context.Context, providerID ProviderID, opts ...RequestOption) ([]ProviderModel, error) {
	return list[ProviderModel]("models")(s.s.send(ctx, op.ListProviderModels(string(providerID)), nil, opts))
}

// ThinkingLevels returns the thinking levels a model accepts on the provider; model is the
// provider's own model ID. It sends listThinkingLevels:
// https://docs.blazingagents.com/api-reference/rest-api/providers#list-thinking-levels
func (s *ProvidersService) ThinkingLevels(ctx context.Context, providerID ProviderID, model string, opts ...RequestOption) (*ProviderThinkingLevels, error) {
	return decode[ProviderThinkingLevels](s.s.send(ctx, op.ListThinkingLevels(string(providerID)), thinkingLevelsQuery{model}, opts))
}
