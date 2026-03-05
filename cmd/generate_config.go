package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CampusTech/retriever2snipe/retriever"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewGenerateConfigCmd creates the generate-config command.
func NewGenerateConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate-config",
		Short: "Generate an example config.yaml with model mappings from Retriever data",
		Long:  "Reads cached Retriever data (or fetches from API) and generates a config.yaml.example file with all unique Retriever model names pre-filled in the model_mappings section.",
		RunE:  runGenerateConfig,
	}

	cmd.Flags().StringP("output", "o", "config.yaml.example", "Output file path")
	cmd.Flags().BoolVar(&Cfg.UseCache, "use-cache", false, "Use cached data instead of fetching from Retriever API")

	return cmd
}

func runGenerateConfig(cmd *cobra.Command, args []string) error {
	outputPath, _ := cmd.Flags().GetString("output")

	// Load devices from cache or API
	var devices []retriever.WarehouseDevice
	var err error

	if Cfg.UseCache {
		devices, err = ReadJSON[[]retriever.WarehouseDevice](filepath.Join(Cfg.CacheDir, "warehouse.json"))
		if err != nil {
			return fmt.Errorf("reading warehouse cache: %w", err)
		}
	} else {
		if Cfg.RetrieverAPIKey == "" {
			return fmt.Errorf("retriever API key is required when not using cache")
		}
		client := NewRetrieverClient()
		devices, err = client.ListAllWarehouseDevices(context.Background())
		if err != nil {
			return fmt.Errorf("fetching warehouse devices: %w", err)
		}
	}

	// Collect unique model names with counts (case-insensitive dedup)
	modelCounts := make(map[string]int)     // lowercase key -> count
	canonicalName := make(map[string]string) // lowercase key -> first-seen casing
	for _, d := range devices {
		name := strings.TrimSpace(fmt.Sprintf("%s %s", d.Manufacturer, d.Model))
		if name == " " || name == "" {
			continue
		}
		lower := strings.ToLower(name)
		modelCounts[lower] += 1
		if _, ok := canonicalName[lower]; !ok {
			canonicalName[lower] = name
		}
	}

	// Sort by canonical name
	var lowerKeys []string
	for k := range modelCounts {
		lowerKeys = append(lowerKeys, k)
	}
	sort.Strings(lowerKeys)

	// Build model_mappings YAML block
	var buf strings.Builder
	buf.WriteString(`# retriever2snipe configuration
# Copy this file to config.yaml and fill in your values.
# CLI flags and environment variables override values set here.

# Retriever API settings
# retriever_api_key: "your-retriever-api-key"  # or set RETRIEVER_API_KEY env var

# Snipe-IT API settings
# snipeit_api_key: "your-snipeit-api-key"  # or set SNIPEIT_API_KEY env var
# snipeit_url: "https://your-instance.snipeitapp.com"  # or set SNIPEIT_URL env var

# Cache settings
# cache_dir: ".cache"  # or set RETRIEVER_CACHE_DIR env var
# use_cache: false

# Sync behavior
# dry_run: false
# debug: false

# Snipe-IT status label IDs
# default_status_id: 0     # (required) Fallback status for unmatched statuses

# Status mappings (Retriever status -> Snipe-IT status label ID)
# Run "retriever2snipe setup" to auto-create all status labels.
# status_mappings:
#   administrative_hold: 0    # undeployable
#   delivered_address: 0      # archived
#   deployed: 0               # deployable
#   deployment_initiated: 0   # pending
#   deployment_requested: 0   # pending
#   device_received: 0        # pending
#   disposal_initiated: 0     # archived
#   disposed: 0               # archived
#   in_repair: 0              # undeployable
#   input_required: 0         # pending
#   label_generated: 0        # pending
#   legal_hold: 0             # undeployable
#   lost_in_transit: 0        # undeployable
#   provisioning: 0           # pending
#   ready_for_deployment: 0   # deployable
#   requires_service: 0       # undeployable
#   returned: 0               # pending
#   return_initiated: 0       # pending
#   returned_to_vendor: 0     # archived
#   retrieval_initiated: 0    # pending
#   to_be_retired: 0          # archived

# Snipe-IT model ID
# default_model_id: 0      # Model ID to assign if no match found

# Snipe-IT location IDs
# warehouse_location_id: 0 # Location ID for Retriever Warehouse

# Custom field mappings (Snipe-IT db column name -> Retriever field name)
# Run "retriever2snipe setup" to auto-create the Retriever-specific fields.
# You can also map Retriever data to any existing Snipe-IT custom field.
# Supported Retriever fields: id, has_charger, codd, legal_hold, ram, disk,
#   os, os_version, screen_size, rating, processor, serial_number, manufacturer,
#   model, device_series, release_year, status, current_location, asset_tag,
#   recipient_source, notes
# RAM is automatically converted to megabytes; disk stays in gigabytes.
# CODD (Certificate of Data Destruction) stores the URL; the PDF is also
# downloaded and attached to the asset when it is a Google Drive link.
# custom_fields:
#   _snipeit_retriever_id_1: id
#   _snipeit_retriever_has_charger_2: has_charger
#   _snipeit_retriever_codd_3: codd
#   _snipeit_retriever_legal_hold_4: legal_hold
#   _snipeit_retriever_rating_5: rating
#   _snipeit_ram_2: ram              # map to your existing Snipe-IT RAM field
#   _snipeit_storage_3: disk         # map to your existing Snipe-IT Storage field

# Model mappings (Retriever "Manufacturer Model" -> Snipe-IT model ID)
# Set the Snipe-IT model ID for each Retriever model name.
# Devices not matching any entry will use default_model_id.
`)

	if len(lowerKeys) > 0 {
		buf.WriteString("model_mappings:\n")
		for _, key := range lowerKeys {
			buf.WriteString(fmt.Sprintf("  %q: 0  # %d device(s)\n", canonicalName[key], modelCounts[key]))
		}
	} else {
		buf.WriteString("# model_mappings:\n#   \"Apple MacBook Air (13-inch) [M4]\": 0\n")
	}

	if err := os.WriteFile(outputPath, []byte(buf.String()), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}

	log.Infof("Generated %s with %d model mappings", outputPath, len(lowerKeys))
	return nil
}
