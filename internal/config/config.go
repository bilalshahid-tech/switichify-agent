package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json or console
}

type AgentConfig struct {
	Name                string   `mapstructure:"name"`
	IntervalSeconds     int      `mapstructure:"interval_seconds"`
	MetricsWindowSec    int      `mapstructure:"metrics_window_seconds"`
	BackendURL          string   `mapstructure:"backend_url"`
	SendIntervalSeconds int      `mapstructure:"send_interval_seconds"`
	TimeoutSeconds      int      `mapstructure:"timeout_seconds"`
	BackendAuthTokenEnv string   `mapstructure:"backend_auth_token_env"` // e.g. SWITCHIFY_BACKEND_TOKEN
	InsecureSkipVerify  bool     `mapstructure:"insecure_skip_verify"`
	MaxQueueSize        int      `mapstructure:"max_queue_size"`
	TestHosts           []string `mapstructure:"test_hosts"`
}

type ISPConfig struct {
	Name                   string   `mapstructure:"name"`
	Interface              string   `mapstructure:"interface"`
	TestHosts              []string `mapstructure:"test_hosts"`
	ICMPThresholdMs        int      `mapstructure:"icmp_threshold_ms"`
	PacketLossThresholdPct int      `mapstructure:"packet_loss_threshold_pct"`
	FailCount              int      `mapstructure:"fail_count"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
}
type Config struct {
	Agent   AgentConfig   `mapstructure:"agent"`
	Primary ISPConfig     `mapstructure:"primary_isp"`
	Backup  ISPConfig     `mapstructure:"backup_isp"`
	Logging LoggingConfig `mapstructure:"logging"`
	Kafka   KafkaConfig   `yaml:"kafka"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// env overrides: SWITCHIFY_AGENT_NAME etc. (optional)
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("agent.interval_seconds", 30)
	v.SetDefault("agent.timeout_seconds", 5)
	v.SetDefault("agent.max_queue_size", 1000)
	v.SetDefault("agent.insecure_skip_verify", false)
	v.SetDefault("agent.metrics_window_seconds", 60)
	v.SetDefault("agent.send_interval_seconds", 30)
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// quick sanity checks
	if cfg.Agent.IntervalSeconds < 1 {
		cfg.Agent.IntervalSeconds = 5
	}
	if cfg.Agent.TimeoutSeconds == 0 {
		cfg.Agent.TimeoutSeconds = 5
	}

	// Example: convert to durations if needed
	_ = time.Duration(cfg.Agent.IntervalSeconds) * time.Second

	return &cfg, nil
}
