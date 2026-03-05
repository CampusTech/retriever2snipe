package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/CampusTech/retriever2snipe/retriever"
	snipeit "github.com/michellepellon/go-snipeit"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config holds application configuration.
type Config struct {
	RetrieverAPIKey string `yaml:"retriever_api_key"`
	SnipeITAPIKey   string `yaml:"snipeit_api_key"`
	SnipeITURL      string `yaml:"snipeit_url"`
	CacheDir        string `yaml:"cache_dir"`
	DryRun          bool   `yaml:"dry_run"`
	Debug           bool   `yaml:"debug"`
	UseCache        bool   `yaml:"use_cache"`

	// Snipe-IT IDs for mapping
	DefaultStatusID     int `yaml:"default_status_id"`
	DefaultModelID      int `yaml:"default_model_id"`
	WarehouseLocationID int `yaml:"warehouse_location_id"`

	// Status mappings (Retriever status string -> Snipe-IT status label ID)
	StatusMappings map[string]int `yaml:"status_mappings"`

	// Custom field mappings (Snipe-IT db column name -> Retriever field)
	CustomFields map[string]string `yaml:"custom_fields"`

	// Model mappings (Retriever "Manufacturer Model" -> Snipe-IT model ID)
	ModelMappings map[string]int `yaml:"model_mappings"`
}

// RetrieverStatusLabel defines a Retriever status with its display name and Snipe-IT type.
type RetrieverStatusLabel struct {
	Key       string // Retriever API status key (e.g. "deployed")
	Label     string // Human-readable label for Snipe-IT (e.g. "Deployed")
	SnipeType string // Snipe-IT status type: deployable, pending, archived, undeployable
}

// AllRetrieverStatuses defines all known Retriever statuses and how they map to Snipe-IT types.
var AllRetrieverStatuses = []RetrieverStatusLabel{
	{"administrative_hold", "Administrative Hold", "undeployable"},
	{"delivered_address", "Delivered For Disposal", "archived"},
	{"deployed", "Deployed", "deployable"},
	{"deployment_initiated", "Deployment In Transit", "pending"},
	{"deployment_requested", "Deployment Requested", "pending"},
	{"device_received", "Device Received", "pending"},
	{"disposal_initiated", "Disposal Initiated", "archived"},
	{"disposed", "Disposed", "archived"},
	{"in_repair", "In Repair", "undeployable"},
	{"input_required", "Input Required", "pending"},
	{"label_generated", "Label Generated", "pending"},
	{"legal_hold", "Legal Hold", "undeployable"},
	{"lost_in_transit", "Lost In Transit", "undeployable"},
	{"provisioning", "Provisioning", "pending"},
	{"ready_for_deployment", "Ready to Deploy", "deployable"},
	{"requires_service", "Requires Service", "undeployable"},
	{"returned", "Returned", "pending"},
	{"return_initiated", "Return Initiated", "pending"},
	{"returned_to_vendor", "Returned To Vendor", "archived"},
	{"retrieval_initiated", "Retrieval In Transit", "pending"},
	{"to_be_retired", "To Be Retired", "archived"},
}

// RetrieverBaseURL is the hardcoded Retriever API base URL.
const RetrieverBaseURL = "https://app.helloretriever.com"

var (
	// Cfg is the global application configuration.
	Cfg Config
	// ConfigFile is the path to the config file.
	ConfigFile string
)

// NewRetrieverClient creates a new Retriever API client using the global config.
func NewRetrieverClient() *retriever.Client {
	return retriever.NewClient(RetrieverBaseURL, Cfg.RetrieverAPIKey)
}

// NewSnipeClient creates a new Snipe-IT client using the global config.
func NewSnipeClient() (*snipeit.Client, error) {
	opts := &snipeit.ClientOptions{
		RateLimiter:    snipeit.NewTokenBucketRateLimiter(2, 5),
		DisableRetries: true,
		Logger:         &snipeLogger{},
	}
	return snipeit.NewClientWithOptions(Cfg.SnipeITURL, Cfg.SnipeITAPIKey, opts)
}

// WriteJSON writes a value as indented JSON to a file.
func WriteJSON(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ReadJSON reads and unmarshals a JSON file.
func ReadJSON[T any](path string) (T, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

// IsNotFound returns true if the error indicates a 404/not-found response.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "404") || strings.Contains(errStr, "not found")
}

// LoadConfig loads config from YAML file, then applies env vars, then lets
// CLI flags (already parsed by cobra) take final precedence.
func LoadConfig(cmd *cobra.Command) error {
	// 1. Load YAML config file (if it exists)
	data, err := os.ReadFile(ConfigFile)
	if err == nil {
		if err := yaml.Unmarshal(data, &Cfg); err != nil {
			return fmt.Errorf("parsing config file %s: %w", ConfigFile, err)
		}
		log.Infof("Loaded config from %s", ConfigFile)
	} else if cmd.Flags().Changed("config") {
		// Only error if the user explicitly specified --config
		return fmt.Errorf("reading config file %s: %w", ConfigFile, err)
	}

	// 2. Env vars override config file values
	applyEnv(&Cfg.RetrieverAPIKey, "RETRIEVER_API_KEY")
	applyEnv(&Cfg.SnipeITAPIKey, "SNIPEIT_API_KEY")
	applyEnv(&Cfg.SnipeITURL, "SNIPEIT_URL")
	applyEnv(&Cfg.CacheDir, "RETRIEVER_CACHE_DIR")

	// 3. CLI flags override everything (cobra already parsed them into Cfg
	//    for flags that were explicitly set; for unset flags we need to avoid
	//    clobbering the config/env values with zero defaults)
	ApplyStringFlag(cmd, "retriever-api-key", &Cfg.RetrieverAPIKey)
	ApplyStringFlag(cmd, "snipeit-api-key", &Cfg.SnipeITAPIKey)
	ApplyStringFlag(cmd, "snipeit-url", &Cfg.SnipeITURL)
	ApplyStringFlag(cmd, "cache-dir", &Cfg.CacheDir)
	ApplyBoolFlag(cmd, "dry-run", &Cfg.DryRun)
	ApplyBoolFlag(cmd, "debug", &Cfg.Debug)
	ApplyBoolFlag(cmd, "use-cache", &Cfg.UseCache)
	ApplyIntFlag(cmd, "default-status-id", &Cfg.DefaultStatusID)
	ApplyIntFlag(cmd, "default-model-id", &Cfg.DefaultModelID)
	ApplyIntFlag(cmd, "warehouse-location-id", &Cfg.WarehouseLocationID)

	// 4. Apply defaults for values still empty
	if Cfg.CacheDir == "" {
		Cfg.CacheDir = ".cache"
	}

	// 5. Configure log level
	if Cfg.Debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	return nil
}

func applyEnv(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// ApplyStringFlag applies a string flag value if it was explicitly set.
func ApplyStringFlag(cmd *cobra.Command, name string, dst *string) {
	if cmd.Flags().Changed(name) {
		*dst, _ = cmd.Flags().GetString(name)
	}
}

// ApplyBoolFlag applies a bool flag value if it was explicitly set.
func ApplyBoolFlag(cmd *cobra.Command, name string, dst *bool) {
	if cmd.Flags().Changed(name) {
		*dst, _ = cmd.Flags().GetBool(name)
	}
}

// ApplyIntFlag applies an int flag value if it was explicitly set.
func ApplyIntFlag(cmd *cobra.Command, name string, dst *int) {
	if cmd.Flags().Changed(name) {
		*dst, _ = cmd.Flags().GetInt(name)
	}
}

// Execute builds the root command, registers subcommands, and runs.
func Execute() {
	rootCmd := &cobra.Command{
		Use:   "retriever2snipe",
		Short: "Sync asset records from Retriever to Snipe-IT",
		Long:  "Retriever2Snipe syncs warehouse inventory and deployment records from the Retriever v2 API to Snipe-IT asset management.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return LoadConfig(cmd)
		},
	}

	rootCmd.PersistentFlags().StringVar(&ConfigFile, "config", "config.yaml", "Path to YAML config file")

	downloadCmd := NewDownloadCmd()
	syncCmd := NewSyncCmd()
	generateConfigCmd := NewGenerateConfigCmd()
	setupCmd := NewSetupCmd()

	// --retriever-api-key: download, sync, generate-config (not setup)
	for _, cmd := range []*cobra.Command{downloadCmd, syncCmd, generateConfigCmd} {
		cmd.Flags().StringVar(&Cfg.RetrieverAPIKey, "retriever-api-key", "", "Retriever API key (env: RETRIEVER_API_KEY)")
	}
	// --snipeit-api-key, --snipeit-url: sync, setup (not download, generate-config)
	for _, cmd := range []*cobra.Command{syncCmd, setupCmd} {
		cmd.Flags().StringVar(&Cfg.SnipeITAPIKey, "snipeit-api-key", "", "Snipe-IT API key (env: SNIPEIT_API_KEY)")
		cmd.Flags().StringVar(&Cfg.SnipeITURL, "snipeit-url", "", "Snipe-IT base URL (env: SNIPEIT_URL)")
	}
	// --cache-dir: download, sync, generate-config (not setup)
	for _, cmd := range []*cobra.Command{downloadCmd, syncCmd, generateConfigCmd} {
		cmd.Flags().StringVar(&Cfg.CacheDir, "cache-dir", "", "Directory for cached API responses")
	}

	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(generateConfigCmd)
	rootCmd.AddCommand(setupCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
