package migrate

// These are decoder-only legacy declarations. No legacy runtime, environment
// expansion, network clients, or notifier constructors are retained here.
type legacyConfig struct {
	Server      legacyServer              `yaml:"server"`
	Notifiers   map[string]legacyNotifier `yaml:"notifiers"`
	Rules       []legacyRule              `yaml:"rules"`
	Persistence struct {
		StateFile     string `yaml:"state_file"`
		FlushInterval string `yaml:"flush_interval"`
	} `yaml:"persistence"`
	AlertLog struct {
		Path string `yaml:"path"`
	} `yaml:"alert_log"`
}
type legacyServer struct {
	Listen        string `yaml:"listen"`
	AdminToken    string `yaml:"admin_token"`
	IngestToken   string `yaml:"ingest_token"`
	TokenFile     string `yaml:"token_file"`
	Port          int    `yaml:"port"`
	Format        string `yaml:"format"`
	MaxBufferSize int    `yaml:"max_buffer_size"`
	MaxLabelSets  int    `yaml:"max_label_sets"`
	StateIdleTTL  string `yaml:"state_idle_ttl"`
	ReadTimeout   string `yaml:"read_timeout"`
	WriteTimeout  string `yaml:"write_timeout"`
	IdleTimeout   string `yaml:"idle_timeout"`
	MaxBodyBytes  int    `yaml:"max_body_bytes"`
	JQ            string `yaml:"jq"`
	DrainTimeout  string `yaml:"drain_timeout"`
}
type legacyNotifier struct {
	Type           string `yaml:"type"`
	URL            string `yaml:"url"`
	Token          string `yaml:"token"`
	ChatID         string `yaml:"chat_id"`
	MaxAttempts    int    `yaml:"max_attempts"`
	InitialBackoff string `yaml:"initial_backoff"`
	Namespace      string `yaml:"namespace"`
	EventReason    string `yaml:"event_reason"`
	EventType      string `yaml:"event_type"`
	Path           string `yaml:"path"`
	Style          string `yaml:"style"`
}
type legacyRule struct {
	Name      string            `yaml:"name"`
	Match     map[string]string `yaml:"match"`
	Condition string            `yaml:"condition"`
	Cooldown  string            `yaml:"cooldown"`
	Message   string            `yaml:"message"`
	Alert     []struct {
		Notifier string `yaml:"notifier"`
	} `yaml:"alert"`
	Guard *struct {
		URL          string `yaml:"url"`
		ExpectStatus int    `yaml:"expect_status"`
		TTL          string `yaml:"ttl"`
	} `yaml:"guard"`
	Mode string `yaml:"mode"`
}
