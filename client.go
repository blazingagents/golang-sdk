package blazingagents

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/blazingagents/golang-sdk/internal/op"
)

const (
	Version               = "0.3.0"
	DefaultBaseURL        = "https://api.blazingagents.com"
	APIKeyEnv             = "BLAZING_AGENTS_API_KEY"
	DefaultTimeout        = 60 * time.Second
	DefaultConnectTimeout = 10 * time.Second
)

// Client is the tenant client. It is safe for concurrent use.
type Client struct {
	Agents              *AgentsService
	Skills              *SkillsService
	Sessions            *SessionsService
	Memories            *MemoriesService
	Prompts             *PromptsService
	Tasks               *TasksService
	Workspaces          *WorkspacesService
	Artifacts           *ArtifactsService
	Chat                *ChatService
	Generation          *GenerationService
	Usage               *UsageService
	Providers           *ProvidersService
	MCPConnections      *MCPConnectionsService
	Tenant              *TenantService
	SpendingLimits      *SpendingLimitsService
	ChatConnections     *ChatConnectionsService
	ChatDeliveries      *ChatDeliveriesService
	MerchantConnection  *MerchantConnectionService
	MerchantBindings    *MerchantBindingsService
	MerchantUsageEvents *MerchantUsageEventsService

	t *transport
}

// UserClient acts for one end user: every request carries X-BA-User-Id. It has only
// the services an end user may call.
type UserClient struct {
	Agents     *AgentsService
	Skills     *SkillsService
	Sessions   *SessionsService
	Memories   *MemoriesService
	Prompts    *PromptsService
	Tasks      *TasksService
	Workspaces *WorkspacesService
	Artifacts  *ArtifactsService
	Chat       *ChatService
	Generation *GenerationService
	Usage      *UserUsageService

	t *transport
}

// userOps is held by every service on both clients; it accepts only op.UserScoped.
// tenantOps is held by tenant-only services; it accepts only op.TenantOnly, so a
// user-scoped operation cannot be left off the UserClient.
type (
	userOps   struct{ t *transport }
	tenantOps struct{ t *transport }
)

// Services on both clients.
type (
	AgentsService     struct{ s userOps }
	SkillsService     struct{ s userOps }
	SessionsService   struct{ s userOps }
	MemoriesService   struct{ s userOps }
	PromptsService    struct{ s userOps }
	TasksService      struct{ s userOps }
	WorkspacesService struct{ s userOps }
	ArtifactsService  struct{ s userOps }
	ChatService       struct{ s userOps }
	GenerationService struct{ s userOps }
	UserUsageService  struct{ s userOps }
)

// Tenant-only services.
type (
	UsageService struct {
		UserUsageService
		s tenantOps
	}
	ProvidersService           struct{ s tenantOps }
	MCPConnectionsService      struct{ s tenantOps }
	TenantService              struct{ s tenantOps }
	SpendingLimitsService      struct{ s tenantOps }
	ChatConnectionsService     struct{ s tenantOps }
	ChatDeliveriesService      struct{ s tenantOps }
	MerchantConnectionService  struct{ s tenantOps }
	MerchantBindingsService    struct{ s tenantOps }
	MerchantUsageEventsService struct{ s tenantOps }
)

func (s userOps) send(ctx context.Context, r op.UserScoped, params any, opts []RequestOption) (*body, error) {
	return s.t.send(ctx, r.Request, params, opts)
}

func (s tenantOps) send(ctx context.Context, r op.TenantOnly, params any, opts []RequestOption) (*body, error) {
	return s.t.send(ctx, r.Request, params, opts)
}

// Option configures NewClient.
type Option func(*config) error

type config struct {
	apiKey            string
	apiKeySet         bool
	baseURL           string
	timeout           time.Duration
	connectTimeout    time.Duration
	connectTimeoutSet bool
	idleTimeout       time.Duration
	roundTripper      http.RoundTripper
	header            http.Header
	userAgent         string
	observer          func(ResponseObservation)
	logger            *slog.Logger
}

// WithAPIKey sets the API key. It wins over BLAZING_AGENTS_API_KEY.
func WithAPIKey(key string) Option {
	return func(c *config) error {
		c.apiKey, c.apiKeySet = key, true
		return nil
	}
}

// WithBaseURL sets the API origin: an absolute http or https URL with no query or fragment.
func WithBaseURL(raw string) Option {
	return func(c *config) error {
		c.baseURL = raw
		return nil
	}
}

// WithTimeout bounds an ordinary call from send to fully read body, and a stream until its
// headers arrive. 0 disables it. The default is 60 seconds.
func WithTimeout(d time.Duration) Option {
	return func(c *config) error {
		c.timeout = d
		return nil
	}
}

// WithConnectTimeout sets the dial timeout of the default transport. The default is 10 seconds.
func WithConnectTimeout(d time.Duration) Option {
	return func(c *config) error {
		c.connectTimeout, c.connectTimeoutSet = d, true
		return nil
	}
}

// WithStreamIdleTimeout bounds the gap between body reads of a stream or download.
// The default is 0, no bound.
func WithStreamIdleTimeout(d time.Duration) Option {
	return func(c *config) error {
		c.idleTimeout = d
		return nil
	}
}

// WithTransport sets the HTTP transport, for proxies, TLS, tracing and test doubles.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) error {
		c.roundTripper = rt
		return nil
	}
}

// WithHeader adds a header to every request. An SDK-owned or invalid header is ErrInvalidHeader.
func WithHeader(name, value string) Option {
	return func(c *config) error {
		if err := checkHeader(name, value); err != nil {
			return fmt.Errorf("%w %s: %s", ErrInvalidHeader, name, err.reason)
		}
		c.header.Add(name, value)
		return nil
	}
}

// WithUserAgent appends ua to the SDK's "blazing-agents-go/<Version>" User-Agent.
func WithUserAgent(ua string) Option {
	return func(c *config) error {
		if !validHeaderValue(ua) {
			return fmt.Errorf("%w User-Agent: not a valid header value", ErrInvalidHeader)
		}
		c.userAgent = ua
		return nil
	}
}

// WithObserver calls fn once per response. A panic in fn is recovered and discarded.
func WithObserver(fn func(ResponseObservation)) Option {
	return func(c *config) error {
		c.observer = fn
		return nil
	}
}

// WithLogger receives one Debug record per response and the function runner's records.
// It never receives bodies, headers or schemas. The default discards.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) error {
		c.logger = l
		return nil
	}
}

var apiKeyPattern = regexp.MustCompile(op.APIKeyPattern)

// NewClient makes a tenant client. It sends no request. The API key comes from WithAPIKey
// or BLAZING_AGENTS_API_KEY and must have the documented format.
func NewClient(opts ...Option) (*Client, error) {
	cfg := config{baseURL: DefaultBaseURL, timeout: DefaultTimeout, connectTimeout: DefaultConnectTimeout,
		header: http.Header{}, logger: slog.New(slog.DiscardHandler)}
	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}
	key, source := cfg.apiKey, "WithAPIKey"
	if !cfg.apiKeySet {
		key, source = os.Getenv(APIKeyEnv), APIKeyEnv
	}
	if key == "" {
		return nil, ErrMissingAPIKey
	}
	if !apiKeyPattern.MatchString(key) {
		return nil, fmt.Errorf("%w: the key from %s is not ba_read_ or ba_full_ followed by 40 letters and digits", ErrInvalidAPIKey, source)
	}
	base, err := parseBaseURL(cfg.baseURL)
	if err != nil {
		return nil, err
	}
	rt := cfg.roundTripper
	switch {
	case rt == nil:
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DialContext = (&net.Dialer{Timeout: cfg.connectTimeout, KeepAlive: 30 * time.Second}).DialContext
		rt = tr
	case cfg.connectTimeoutSet:
		return nil, fmt.Errorf("%w: WithConnectTimeout applies to the default transport; set dial timeouts on the WithTransport value", ErrConflictingOptions)
	}
	logger := cfg.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	ua := "blazing-agents-go/" + Version
	if cfg.userAgent != "" {
		ua += " " + cfg.userAgent
	}
	return newClient(&transport{
		http: &http.Client{Transport: rt, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		baseURL: base, authorization: NewSecret("Bearer " + key), userAgent: ua, header: cfg.header,
		timeout: cfg.timeout, idleTimeout: cfg.idleTimeout, observer: cfg.observer, logger: logger,
	}), nil
}

func parseBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return "", fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	case u.Scheme != "http" && u.Scheme != "https" || u.Host == "":
		return "", fmt.Errorf("%w: %q is not an absolute http or https URL", ErrInvalidBaseURL, raw)
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "":
		return "", fmt.Errorf("%w: %q has a query or fragment", ErrInvalidBaseURL, raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func newClient(t *transport) *Client {
	u, ten := userOps{t}, tenantOps{t}
	return &Client{
		Agents:              &AgentsService{u},
		Skills:              &SkillsService{u},
		Sessions:            &SessionsService{u},
		Memories:            &MemoriesService{u},
		Prompts:             &PromptsService{u},
		Tasks:               &TasksService{u},
		Workspaces:          &WorkspacesService{u},
		Artifacts:           &ArtifactsService{u},
		Chat:                &ChatService{u},
		Generation:          &GenerationService{u},
		Usage:               &UsageService{UserUsageService{u}, ten},
		Providers:           &ProvidersService{ten},
		MCPConnections:      &MCPConnectionsService{ten},
		Tenant:              &TenantService{ten},
		SpendingLimits:      &SpendingLimitsService{ten},
		ChatConnections:     &ChatConnectionsService{ten},
		ChatDeliveries:      &ChatDeliveriesService{ten},
		MerchantConnection:  &MerchantConnectionService{ten},
		MerchantBindings:    &MerchantBindingsService{ten},
		MerchantUsageEvents: &MerchantUsageEventsService{ten},
		t:                   t,
	}
}

func newUserClient(t *transport) *UserClient {
	u := userOps{t}
	return &UserClient{
		Agents:     &AgentsService{u},
		Skills:     &SkillsService{u},
		Sessions:   &SessionsService{u},
		Memories:   &MemoriesService{u},
		Prompts:    &PromptsService{u},
		Tasks:      &TasksService{u},
		Workspaces: &WorkspacesService{u},
		Artifacts:  &ArtifactsService{u},
		Chat:       &ChatService{u},
		Generation: &GenerationService{u},
		Usage:      &UserUsageService{u},
		t:          t,
	}
}

// printablePattern is printable ASCII with no leading or trailing space: a header value net/http
// sends unchanged, so a user ID or client request ID is reported exactly as it went on the wire.
var printablePattern = regexp.MustCompile(`^[\x21-\x7E](?:[\x20-\x7E]*[\x21-\x7E])?$`)

// ForUser returns a client that acts for userID: 1 to 256 printable ASCII characters with
// no leading or trailing space. It shares the connection pool.
func (c *Client) ForUser(userID string) (*UserClient, error) {
	switch {
	case len(userID) > 256:
		return nil, fmt.Errorf("%w: longer than 256 characters", ErrInvalidUserID)
	case !printablePattern.MatchString(userID):
		return nil, fmt.Errorf("%w: must be printable ASCII with no leading or trailing space", ErrInvalidUserID)
	}
	t := *c.t
	t.userID = userID
	return newUserClient(&t), nil
}

const requestIDRule = "must be printable ASCII with no leading or trailing space"

// validRequestID reports whether id is "" or a value that goes on the wire unchanged.
func validRequestID(id string) bool { return id == "" || printablePattern.MatchString(id) }

// WithClientRequestID returns a copy that sends X-Client-Request-Id on every request.
// "" clears it. The copy shares the connection pool. The ID must be printable ASCII with no
// leading or trailing space.
func (c *Client) WithClientRequestID(id string) (*Client, error) {
	t, err := c.t.withClientRequestID(id)
	if err != nil {
		return nil, err
	}
	return newClient(t), nil
}

// WithClientRequestID returns a copy that sends X-Client-Request-Id on every request.
// "" clears it. The copy shares the connection pool.
func (u *UserClient) WithClientRequestID(id string) (*UserClient, error) {
	t, err := u.t.withClientRequestID(id)
	if err != nil {
		return nil, err
	}
	return newUserClient(t), nil
}

func (t *transport) withClientRequestID(id string) (*transport, error) {
	if !validRequestID(id) {
		return nil, fmt.Errorf("%w X-Client-Request-Id: %s", ErrInvalidHeader, requestIDRule)
	}
	c := *t
	c.clientRequestID = id
	return &c, nil
}
