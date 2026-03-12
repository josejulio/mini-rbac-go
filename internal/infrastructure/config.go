package infrastructure

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Kessel   KesselConfig
	App      AppConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	HTTP HTTPConfig
	GRPC GRPCConfig
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Addr    string
	Timeout time.Duration
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Addr    string
	Timeout time.Duration
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string        `mapstructure:"dbname"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// KesselConfig holds Kessel relations-api configuration
type KesselConfig struct {
	RelationsAPI RelationsAPIConfig `mapstructure:"relations_api"`
}

// RelationsAPIConfig holds relations-api client configuration
type RelationsAPIConfig struct {
	Host       string
	Port       int
	TLSEnabled bool          `mapstructure:"tls_enabled"`
	Timeout    time.Duration
}

// AppConfig holds general application configuration
type AppConfig struct {
	Name               string
	Version            string
	Env                string
	ReplicationEnabled bool `mapstructure:"replication_enabled"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Allow environment variables to override config
	v.AutomaticEnv()

	// Unmarshal config
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// DSN returns the PostgreSQL connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// RelationsAPIAddress returns the full address for the relations-api
func (c *RelationsAPIConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
