package web

import "time"

const (
	stateAvailable   = "available"
	stateUnavailable = "unavailable"
	matchExact       = "exact"
	ipv6Loopback     = "::1"
)

// ContextProjection contains only display-safe context metadata.
type ContextProjection struct {
	Name              string `json:"name"`
	ServerIdentity    string `json:"serverIdentity"`
	MCPServerIdentity string `json:"mcpServerIdentity"`
	ServerState       string `json:"serverState"`
	MCPServerState    string `json:"mcpServerState"`
	Current           bool   `json:"current"`
}

// ContextListResponse contains configured context projections.
type ContextListResponse struct {
	Items      []ContextProjection `json:"items"`
	ObservedAt time.Time           `json:"observedAt"`
	Stale      bool                `json:"stale,omitempty"`
}

// ConsoleState describes the local console process.
type ConsoleState struct {
	State string `json:"state"`
}

// DeploymentResponse contains one safe deployment projection.
type DeploymentResponse struct {
	Context    ContextProjection `json:"context"`
	Console    ConsoleState      `json:"console"`
	ObservedAt time.Time         `json:"observedAt"`
	Stale      bool              `json:"stale,omitempty"`
}

// ServerInfoResponse contains safe optional server metadata.
type ServerInfoResponse struct {
	State           string    `json:"state"`
	Version         string    `json:"version,omitempty"`
	Commit          string    `json:"commit,omitempty"`
	Date            string    `json:"date,omitempty"`
	CacheBackend    string    `json:"cacheBackend,omitempty"`
	ActiveToolCount int       `json:"activeToolCount,omitempty"`
	ObservedAt      time.Time `json:"observedAt"`
}

// LogListResponse contains projected log events.
type LogListResponse struct {
	State string     `json:"state"`
	Items []LogEvent `json:"items"`
}

// HealthResponse contains a safe health projection.
type HealthResponse struct {
	State      string    `json:"state"`
	Status     string    `json:"status,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

// CacheStatsProjection contains safe cache counters.
type CacheStatsProjection struct {
	ExactKeys   int64  `json:"exactKeys"`
	RedisMemory string `json:"redisMemory,omitempty"`
}

// CacheResponse contains a safe cache projection.
type CacheResponse struct {
	State      string                `json:"state"`
	Stats      *CacheStatsProjection `json:"stats,omitempty"`
	ObservedAt time.Time             `json:"observedAt"`
}

// ToolProjection contains tool metadata without credentials or full URLs.
type ToolProjection struct {
	Name         string                  `json:"name"`
	Version      string                  `json:"version"`
	Module       string                  `json:"module,omitempty"`
	Status       string                  `json:"status,omitempty"`
	Active       bool                    `json:"active"`
	Description  string                  `json:"description,omitempty"`
	Method       string                  `json:"method,omitempty"`
	EndpointPath string                  `json:"endpointPath,omitempty"`
	ResponsePath string                  `json:"responsePath,omitempty"`
	AllowedRoles []string                `json:"allowedRoles,omitempty"`
	Cache        *CacheProjection        `json:"cache,omitempty"`
	Lifecycle    *LifecycleProjection    `json:"lifecycle,omitempty"`
	Manifest     *ToolManifestProjection `json:"manifest,omitempty"`
}

// ToolManifestProjection contains the user-facing, credential-free parts of a
// tool manifest. It intentionally omits credential references and raw schemas.
type ToolManifestProjection struct {
	APIVersion  string                     `json:"apiVersion,omitempty"`
	Kind        string                     `json:"kind,omitempty"`
	Description ToolDescriptionProjection  `json:"description"`
	InputType   string                     `json:"inputType,omitempty"`
	InputFields []ToolInputFieldProjection `json:"inputFields,omitempty"`
	OutputType  string                     `json:"outputType,omitempty"`
	Execution   ToolExecutionProjection    `json:"execution"`
	Security    ToolSecurityProjection     `json:"security"`
	Routing     *ToolRoutingProjection     `json:"routing,omitempty"`
}

// ToolDescriptionProjection contains the descriptive guidance safe to show in
// the console.
type ToolDescriptionProjection struct {
	Short        string   `json:"short,omitempty"`
	WhenToUse    []string `json:"whenToUse,omitempty"`
	WhenNotToUse []string `json:"whenNotToUse,omitempty"`
	Examples     []string `json:"examples,omitempty"`
}

// ToolInputFieldProjection describes one input without exposing default values.
type ToolInputFieldProjection struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Required    bool     `json:"required"`
}

// ToolExecutionProjection contains only the safe execution mapping.
type ToolExecutionProjection struct {
	Type         string            `json:"type,omitempty"`
	Method       string            `json:"method,omitempty"`
	EndpointPath string            `json:"endpointPath,omitempty"`
	ResponsePath string            `json:"responsePath,omitempty"`
	Mapping      map[string]string `json:"mapping,omitempty"`
}

// ToolSecurityProjection omits the credential reference.
type ToolSecurityProjection struct {
	AuthType     string   `json:"authType,omitempty"`
	AllowedRoles []string `json:"allowedRoles,omitempty"`
}

// ToolRoutingProjection contains non-sensitive routing hints.
type ToolRoutingProjection struct {
	Priority    float64  `json:"priority"`
	Signals     []string `json:"signals,omitempty"`
	AntiSignals []string `json:"antiSignals,omitempty"`
}

// CacheProjection contains safe per-tool cache settings.
type CacheProjection struct {
	Enabled    bool `json:"enabled"`
	TTLSeconds int  `json:"ttlSeconds"`
	IsReadOnly bool `json:"isReadOnly"`
}

// LifecycleProjection contains safe lifecycle metadata.
type LifecycleProjection struct {
	Status       string `json:"status"`
	DeprecatedAt string `json:"deprecatedAt,omitempty"`
	SunsetAt     string `json:"sunsetAt,omitempty"`
	Replacement  string `json:"replacement,omitempty"`
}

// ToolListResponse contains safe tool projections.
type ToolListResponse struct {
	State      string           `json:"state"`
	Items      []ToolProjection `json:"items"`
	ObservedAt time.Time        `json:"observedAt"`
}

// APIErrorResponse is the stable local error shape.
type APIErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
