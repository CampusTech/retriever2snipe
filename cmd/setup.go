package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	snipeit "github.com/michellepellon/go-snipeit"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewSetupCmd creates the setup command.
func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create Snipe-IT resources for retriever2snipe and save config",
		Long:  "Creates the warehouse location, en-route status labels, and custom fields in Snipe-IT, then saves the resulting IDs into a config YAML file.",
		RunE:  runSetup,
	}

	cmd.Flags().StringP("output", "o", "", "Output config file path (default: same as --config)")
	cmd.Flags().BoolVar(&Cfg.DryRun, "dry-run", false, "Preview what would be created without making changes")

	return cmd
}

func runSetup(cmd *cobra.Command, args []string) error {
	if Cfg.SnipeITAPIKey == "" {
		return fmt.Errorf("snipe-IT API key is required (--snipeit-api-key or SNIPEIT_API_KEY)")
	}
	if Cfg.SnipeITURL == "" {
		return fmt.Errorf("snipe-IT URL is required (--snipeit-url or SNIPEIT_URL)")
	}

	outputPath, _ := cmd.Flags().GetString("output")
	if outputPath == "" {
		outputPath = ConfigFile
	}

	snipeClient, err := NewSnipeClient()
	if err != nil {
		return fmt.Errorf("creating snipe-IT client: %w", err)
	}

	dryRun := Cfg.DryRun
	if dryRun {
		log.Info("DRY RUN: will search for existing resources but not create anything")
	}

	scanner := bufio.NewScanner(os.Stdin)

	// 1. Create or find warehouse location
	log.Info("Setting up warehouse location...")
	locationID, err := setupLocation(snipeClient, dryRun)
	if err != nil {
		return fmt.Errorf("setting up warehouse location: %w", err)
	}
	Cfg.WarehouseLocationID = locationID
	log.Infof("  Warehouse location ID: %d", locationID)

	// 2. Create or find all Retriever status labels
	log.Info("Setting up status labels...")
	if Cfg.StatusMappings == nil {
		Cfg.StatusMappings = make(map[string]int)
	}
	for _, sl := range AllRetrieverStatuses {
		id, err := setupStatusLabel(snipeClient, sl.Label, sl.SnipeType, dryRun)
		if err != nil {
			return fmt.Errorf("setting up status label %q: %w", sl.Label, err)
		}
		Cfg.StatusMappings[sl.Key] = id
		log.Infof("  %s (%s): ID %d", sl.Label, sl.Key, id)
	}

	// 3. Find or create custom fields
	log.Info("Setting up custom fields...")
	fieldMap, err := setupCustomFields(snipeClient, dryRun)
	if err != nil {
		return fmt.Errorf("setting up custom fields: %w", err)
	}
	Cfg.CustomFields = fieldMap
	for dbCol, retrieverField := range fieldMap {
		log.Infof("  %s -> %s", dbCol, retrieverField)
	}

	// 4. Pick or create fieldset, then associate fields
	if !dryRun {
		log.Info("Setting up fieldset...")
		fieldsetID, err := pickOrCreateFieldset(snipeClient, scanner)
		if err != nil {
			return fmt.Errorf("setting up fieldset: %w", err)
		}
		log.Infof("  Using fieldset ID: %d", fieldsetID)

		// Associate all custom fields with the chosen fieldset
		if err := associateFieldsWithFieldset(snipeClient, fieldMap, fieldsetID); err != nil {
			return fmt.Errorf("associating fields with fieldset: %w", err)
		}
	} else {
		log.Info("DRY RUN: skipping fieldset selection and field association")
	}

	// 5. Write config
	if !dryRun {
		log.Infof("Writing config to %s...", outputPath)
		if err := writeConfig(outputPath); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
	} else {
		log.Info("DRY RUN: would write config to", outputPath)
	}

	log.Info("Setup complete!")
	return nil
}

// setupLocation finds or creates the Retriever Warehouse location.
func setupLocation(client *snipeit.Client, dryRun bool) (int, error) {
	// Search for existing
	list, _, err := client.Locations.List(&snipeit.ListOptions{Search: "Retriever Warehouse", Limit: 50})
	if err != nil {
		return 0, fmt.Errorf("searching locations: %w", err)
	}
	for _, row := range list.Rows {
		if row.Name == "Retriever Warehouse" {
			log.Infof("  Found existing location: %s (ID %d)", row.Name, row.ID)
			return row.ID, nil
		}
	}

	if dryRun {
		log.Info("  DRY RUN: would create location 'Retriever Warehouse'")
		return 0, nil
	}

	// Create new
	resp, _, err := client.Locations.Create(snipeit.Location{
		CommonFields: snipeit.CommonFields{Name: "Retriever Warehouse"},
		Address:      "455 Fairway Drive",
		Address2:     "Suite 100",
		City:         "Deerfield Beach",
		State:        "Florida",
		Country:      "United States",
		Zip:          "33441-1809",
	})
	if err != nil {
		return 0, fmt.Errorf("creating location: %w", err)
	}
	if resp.Status != "success" {
		return 0, fmt.Errorf("creating location: status=%s message=%s", resp.Status, resp.Message)
	}
	log.Infof("  Created location: %s (ID %d)", resp.Payload.Name, resp.Payload.ID)
	return resp.Payload.ID, nil
}

// setupStatusLabel finds or creates a status label by exact name with the given Snipe-IT type.
func setupStatusLabel(client *snipeit.Client, name, statusType string, dryRun bool) (int, error) {
	// Search for existing
	list, _, err := client.StatusLabels.List(&snipeit.ListOptions{Search: name, Limit: 50})
	if err != nil {
		return 0, fmt.Errorf("searching status labels: %w", err)
	}
	for _, row := range list.Rows {
		if row.Name == name {
			log.Infof("  Found existing status label: %s (ID %d)", row.Name, row.ID)
			return row.ID, nil
		}
	}

	if dryRun {
		log.Infof("  DRY RUN: would create status label %q (%s)", name, statusType)
		return 0, nil
	}

	// Create new
	resp, _, err := client.StatusLabels.Create(snipeit.StatusLabel{
		CommonFields: snipeit.CommonFields{Name: name},
		Type:         statusType,
	})
	if err != nil {
		return 0, fmt.Errorf("creating status label: %w", err)
	}
	if resp.Status != "success" {
		return 0, fmt.Errorf("creating status label: status=%s message=%s", resp.Status, resp.Message)
	}
	log.Infof("  Created status label: %s (ID %d)", resp.Payload.Name, resp.Payload.ID)
	return resp.Payload.ID, nil
}

// setupCustomFields finds or creates custom fields needed by retriever2snipe.
// Returns the custom_fields map (db_column_name -> retriever field name).
func setupCustomFields(client *snipeit.Client, dryRun bool) (map[string]string, error) {
	// Get all existing custom fields
	list, _, err := client.Fields.List(&snipeit.ListOptions{Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("listing fields: %w", err)
	}

	// Build lookup by name
	existingByName := make(map[string]struct {
		id       int
		dbColumn string
	})
	for _, row := range list.Rows {
		existingByName[row.Name] = struct {
			id       int
			dbColumn string
		}{row.ID, row.DBColumnName}
	}

	result := make(map[string]string)

	// Fields we need and their retriever mapping
	needed := []struct {
		snipeName      string
		retrieverField string
		dbSlug         string // used for db_column_name fallback when API returns empty; defaults to lowercase snipeName
		element        string
		format         string
		helpText       string
	}{
		{"Retriever: ID", "id", "retriever_id", "text", "", "Retriever device ID"},
		{"Retriever: Has Charger?", "has_charger", "retriever_has_charger", "text", "BOOLEAN", ""},
		{"Retriever: Certificate of Data Destruction", "codd", "retriever_codd", "text", "URL", ""},
		{"Retriever: Legal Hold", "legal_hold", "retriever_legal_hold", "text", "numeric", "Number of days for device to be on legal hold"},
		{"Retriever: Rating", "rating", "retriever_rating", "text", "", "Device condition rating from Retriever"},
	}

	for _, f := range needed {
		if existing, ok := existingByName[f.snipeName]; ok {
			log.Infof("  Found existing field: %s (ID %d, column %s)", f.snipeName, existing.id, existing.dbColumn)
			result[existing.dbColumn] = f.retrieverField
			continue
		}

		if dryRun {
			log.Infof("  DRY RUN: would create field %q (element=%s, format=%s)", f.snipeName, f.element, f.format)
			continue
		}

		// Create the field
		field := snipeit.Field{
			CommonFields: snipeit.CommonFields{Name: f.snipeName},
			Element:      f.element,
			Format:       f.format,
			HelpText:     f.helpText,
		}
		resp, _, err := client.Fields.Create(field)
		if err != nil {
			return nil, fmt.Errorf("creating field %q: %w", f.snipeName, err)
		}
		if resp.Status != "success" {
			return nil, fmt.Errorf("creating field %q: status=%s message=%s", f.snipeName, resp.Status, resp.Message)
		}
		dbCol := resp.Payload.DBColumnName
		if dbCol == "" {
			// Snipe-IT may not return db_column_name in the create response;
			// construct it from the convention: _snipeit_{slug}_{id}
			slug := f.dbSlug
			if slug == "" {
				slug = strings.ToLower(strings.ReplaceAll(strings.TrimSuffix(f.snipeName, "?"), " ", "_"))
			}
			dbCol = fmt.Sprintf("_snipeit_%s_%d", slug, resp.Payload.ID)
		}
		log.Infof("  Created field: %s (ID %d, column %s)", f.snipeName, resp.Payload.ID, dbCol)
		result[dbCol] = f.retrieverField
	}

	return result, nil
}

// pickOrCreateFieldset prompts the user to choose an existing fieldset or create a new one.
func pickOrCreateFieldset(client *snipeit.Client, scanner *bufio.Scanner) (int, error) {
	list, _, err := client.Fieldsets.List(&snipeit.ListOptions{Limit: 500})
	if err != nil {
		return 0, fmt.Errorf("listing fieldsets: %w", err)
	}

	fmt.Println("\nAvailable fieldsets:")
	for i, row := range list.Rows {
		fmt.Printf("  %d) %s (ID %d)\n", i+1, row.Name, row.ID)
	}
	fmt.Printf("  %d) Create new fieldset\n", len(list.Rows)+1)
	fmt.Print("\nChoose a fieldset: ")

	if !scanner.Scan() {
		return 0, fmt.Errorf("no input received")
	}
	choice, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || choice < 1 || choice > len(list.Rows)+1 {
		return 0, fmt.Errorf("invalid choice: %s", scanner.Text())
	}

	if choice <= len(list.Rows) {
		selected := list.Rows[choice-1]
		return selected.ID, nil
	}

	// Create new fieldset
	fmt.Print("Enter new fieldset name: ")
	if !scanner.Scan() {
		return 0, fmt.Errorf("no input received")
	}
	name := strings.TrimSpace(scanner.Text())
	if name == "" {
		return 0, fmt.Errorf("fieldset name cannot be empty")
	}

	resp, _, err := client.Fieldsets.Create(snipeit.Fieldset{
		CommonFields: snipeit.CommonFields{Name: name},
	})
	if err != nil {
		return 0, fmt.Errorf("creating fieldset: %w", err)
	}
	if resp.Status != "success" {
		return 0, fmt.Errorf("creating fieldset: status=%s message=%s", resp.Status, resp.Message)
	}
	log.Infof("  Created fieldset: %s (ID %d)", resp.Payload.Name, resp.Payload.ID)
	return resp.Payload.ID, nil
}

// associateFieldsWithFieldset associates custom fields with a fieldset.
func associateFieldsWithFieldset(client *snipeit.Client, fieldMap map[string]string, fieldsetID int) error {
	// We need the field IDs, but we only have db_column_names in fieldMap.
	// Fetch all fields again to get IDs.
	list, _, err := client.Fields.List(&snipeit.ListOptions{Limit: 500})
	if err != nil {
		return fmt.Errorf("listing fields: %w", err)
	}

	dbColToID := make(map[string]int)
	for _, row := range list.Rows {
		dbColToID[row.DBColumnName] = row.ID
	}

	for dbCol := range fieldMap {
		fieldID, ok := dbColToID[dbCol]
		if !ok {
			log.Warnf("Could not find field ID for %s, skipping association", dbCol)
			continue
		}
		_, err := client.Fields.Associate(fieldID, fieldsetID)
		if err != nil {
			return fmt.Errorf("associating field %s (ID %d) with fieldset %d: %w", dbCol, fieldID, fieldsetID, err)
		}
		log.Infof("  Associated field %s (ID %d) with fieldset %d", dbCol, fieldID, fieldsetID)
	}

	return nil
}

// writeConfig writes the current config to a YAML file, preserving existing values.
func writeConfig(path string) error {
	// Load existing config file if it exists, so we don't lose values we don't manage
	existing := make(map[string]interface{})
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// Update with values from setup
	if Cfg.SnipeITAPIKey != "" {
		existing["snipeit_api_key"] = Cfg.SnipeITAPIKey
	}
	if Cfg.SnipeITURL != "" {
		existing["snipeit_url"] = Cfg.SnipeITURL
	}
	if Cfg.RetrieverAPIKey != "" {
		existing["retriever_api_key"] = Cfg.RetrieverAPIKey
	}
	if Cfg.WarehouseLocationID != 0 {
		existing["warehouse_location_id"] = Cfg.WarehouseLocationID
	}
	if Cfg.DefaultStatusID != 0 {
		existing["default_status_id"] = Cfg.DefaultStatusID
	}
	if len(Cfg.StatusMappings) > 0 {
		existing["status_mappings"] = Cfg.StatusMappings
	}
	if Cfg.DefaultModelID != 0 {
		existing["default_model_id"] = Cfg.DefaultModelID
	}
	if len(Cfg.CustomFields) > 0 {
		existing["custom_fields"] = Cfg.CustomFields
	}
	if len(Cfg.ModelMappings) > 0 {
		existing["model_mappings"] = Cfg.ModelMappings
	}
	if Cfg.UseCache {
		existing["use_cache"] = Cfg.UseCache
	}
	if Cfg.DryRun {
		existing["dry_run"] = Cfg.DryRun
	}

	out, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}
