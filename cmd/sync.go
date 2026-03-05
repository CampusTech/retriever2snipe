package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/CampusTech/retriever2snipe/retriever"
	snipeit "github.com/michellepellon/go-snipeit"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewSyncCmd creates the sync command.
func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync Retriever records to Snipe-IT",
		Long:  "Syncs warehouse device inventory and deployment data from Retriever to Snipe-IT assets. Uses serial number as the primary matching key. Use --serial or --retriever-device-id to sync a single device.",
		RunE:  runSync,
	}

	cmd.Flags().String("serial", "", "Sync only the device with this serial number")
	cmd.Flags().String("retriever-device-id", "", "Sync only the device with this Retriever device ID")
	cmd.Flags().BoolVar(&Cfg.DryRun, "dry-run", false, "Preview changes without writing to Snipe-IT")
	cmd.Flags().BoolVar(&Cfg.Debug, "debug", false, "Show JSON payloads during dry runs")
	cmd.Flags().BoolVar(&Cfg.UseCache, "use-cache", false, "Use cached data instead of fetching from Retriever API")
	cmd.Flags().IntVar(&Cfg.DefaultStatusID, "default-status-id", 0, "Snipe-IT status label ID for new assets")
	cmd.Flags().IntVar(&Cfg.DefaultModelID, "default-model-id", 0, "Snipe-IT default model ID")
	cmd.Flags().IntVar(&Cfg.WarehouseLocationID, "warehouse-location-id", 0, "Snipe-IT location ID for Retriever Warehouse")

	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	if Cfg.SnipeITAPIKey == "" {
		return fmt.Errorf("snipe-IT API key is required (--snipeit-api-key or SNIPEIT_API_KEY)")
	}
	if Cfg.SnipeITURL == "" {
		return fmt.Errorf("snipe-IT URL is required (--snipeit-url or SNIPEIT_URL)")
	}
	if Cfg.DefaultStatusID == 0 {
		return fmt.Errorf("--default-status-id is required")
	}

	// Load Retriever data (from cache or API)
	var devices []retriever.WarehouseDevice
	var deployments []retriever.Deployment
	var returns []retriever.DeviceReturn
	var err error

	if Cfg.UseCache {
		log.Info("Loading data from cache...")
		devices, err = ReadJSON[[]retriever.WarehouseDevice](filepath.Join(Cfg.CacheDir, "warehouse.json"))
		if err != nil {
			return fmt.Errorf("reading warehouse cache: %w", err)
		}
		deployments, err = ReadJSON[[]retriever.Deployment](filepath.Join(Cfg.CacheDir, "deployments.json"))
		if err != nil {
			return fmt.Errorf("reading deployments cache: %w", err)
		}
		returns, err = ReadJSON[[]retriever.DeviceReturn](filepath.Join(Cfg.CacheDir, "device_returns.json"))
		if err != nil {
			return fmt.Errorf("reading device returns cache: %w", err)
		}
	} else {
		if Cfg.RetrieverAPIKey == "" {
			return fmt.Errorf("retriever API key is required when not using cache")
		}
		client := NewRetrieverClient()
		ctx := context.Background()

		log.Info("Fetching warehouse devices from API...")
		devices, err = client.ListAllWarehouseDevices(ctx)
		if err != nil {
			return fmt.Errorf("fetching warehouse devices: %w", err)
		}

		log.Info("Fetching deployments from API...")
		deployments, err = client.ListAllDeployments(ctx)
		if err != nil {
			return fmt.Errorf("fetching deployments: %w", err)
		}

		log.Info("Fetching device returns from API...")
		returns, err = client.ListAllDeviceReturns(ctx)
		if err != nil {
			return fmt.Errorf("fetching device returns: %w", err)
		}
	}

	log.Infof("Loaded %d warehouse devices, %d deployments, %d device returns", len(devices), len(deployments), len(returns))

	// Filter to a single device if --serial or --device-id is provided
	filterSerial, _ := cmd.Flags().GetString("serial")
	filterDeviceID, _ := cmd.Flags().GetString("retriever-device-id")
	if filterSerial != "" || filterDeviceID != "" {
		var filtered []retriever.WarehouseDevice
		for _, d := range devices {
			if (filterSerial != "" && strings.EqualFold(d.SerialNumber, filterSerial)) ||
				(filterDeviceID != "" && d.ID == filterDeviceID) {
				filtered = append(filtered, d)
				break
			}
		}
		if len(filtered) == 0 {
			identifier := filterSerial
			if identifier == "" {
				identifier = filterDeviceID
			}
			return fmt.Errorf("device not found in Retriever data: %s", identifier)
		}
		devices = filtered
		log.Infof("Filtered to single device: %s (serial: %s)", devices[0].ID, devices[0].SerialNumber)
	}

	// Deduplicate by serial number — keep the record with the latest initial_deployment.
	// The same physical device can appear multiple times in the warehouse if it was
	// deployed, returned, and re-deployed.
	seen := make(map[string]int) // serial -> index in deduped slice
	var deduped []retriever.WarehouseDevice
	for _, d := range devices {
		if d.SerialNumber == "" {
			continue
		}
		if idx, ok := seen[d.SerialNumber]; ok {
			existing := deduped[idx]
			// Keep the one with the later initial_deployment
			existingDeploy := ""
			if existing.InitialDeployment != nil {
				existingDeploy = *existing.InitialDeployment
			}
			newDeploy := ""
			if d.InitialDeployment != nil {
				newDeploy = *d.InitialDeployment
			}
			if newDeploy > existingDeploy {
				deduped[idx] = d
			}
		} else {
			seen[d.SerialNumber] = len(deduped)
			deduped = append(deduped, d)
		}
	}
	if len(deduped) < len(devices) {
		log.Infof("Deduplicated %d devices to %d unique serials", len(devices), len(deduped))
	}
	devices = deduped

	// Build deployment lookup by device ID
	deploymentByDeviceID := make(map[string]retriever.Deployment)
	for _, d := range deployments {
		deploymentByDeviceID[d.DeviceID] = d
	}

	// Build device return lookup by serial number (most recent return wins)
	returnBySerial := make(map[string]retriever.DeviceReturn)
	for _, r := range returns {
		if r.SerialNumber == nil || *r.SerialNumber == "" {
			continue
		}
		serial := *r.SerialNumber
		existing, ok := returnBySerial[serial]
		if !ok || r.CreatedAt > existing.CreatedAt {
			returnBySerial[serial] = r
		}
	}

	// Initialize Snipe-IT client
	snipeClient, err := NewSnipeClient()
	if err != nil {
		return fmt.Errorf("creating snipe-IT client: %w", err)
	}

	syncer := &syncer{
		snipe:                snipeClient,
		deploymentByDeviceID: deploymentByDeviceID,
		returnBySerial:       returnBySerial,
		cfg:                  Cfg,
	}

	return syncer.syncAll(devices)
}

// syncer handles the sync logic between Retriever and Snipe-IT.
type syncer struct {
	snipe                *snipeit.Client
	deploymentByDeviceID map[string]retriever.Deployment
	returnBySerial       map[string]retriever.DeviceReturn
	cfg                  Config

	stats struct {
		created int
		updated int
		skipped int
		errors  int
	}
}

func (s *syncer) syncAll(devices []retriever.WarehouseDevice) error {
	for i, device := range devices {
		if device.SerialNumber == "" {
			log.Infof("[%d/%d] Skipping device %s: no serial number", i+1, len(devices), device.ID)
			s.stats.skipped++
			continue
		}

		if err := s.syncDevice(device, i+1, len(devices)); err != nil {
			log.Errorf("[%d/%d] Error syncing device %s (serial: %s): %v", i+1, len(devices), device.ID, device.SerialNumber, err)
			s.stats.errors++
		}
	}

	log.Infof("Sync complete: %d created, %d updated, %d skipped, %d errors",
		s.stats.created, s.stats.updated, s.stats.skipped, s.stats.errors)
	return nil
}

func (s *syncer) syncDevice(device retriever.WarehouseDevice, idx, total int) error {
	serial := device.SerialNumber

	// Look up existing asset in Snipe-IT by serial number
	existing, _, err := s.snipe.Assets.GetAssetBySerial(serial)
	if err != nil {
		// If it's a 404/not-found, we'll create; otherwise it's an error
		if !IsNotFound(err) {
			return fmt.Errorf("looking up serial %s: %w", serial, err)
		}
	}

	var assetID int

	if existing != nil && existing.Total > 0 && len(existing.Rows) > 0 {
		// Update existing asset — preserve existing notes
		existingAsset := existing.Rows[0]
		assetID = existingAsset.ID
		asset := s.mapDeviceToAsset(device, existingAsset.Name, existingAsset.Notes)
		needsUpdate, reason := assetNeedsUpdate(existingAsset, asset)
		if !needsUpdate {
			log.Infof("[%d/%d] Skipping asset %d (serial: %s) — no changes", idx, total, existingAsset.ID, serial)
			s.stats.skipped++
			return nil
		}
		log.Debugf("[%d/%d] Asset %d needs update: %s", idx, total, existingAsset.ID, reason)
		if s.cfg.DryRun {
			log.Infof("[%d/%d] DRY RUN: Would update asset %d (serial: %s)", idx, total, existingAsset.ID, serial)
			s.debugJSON("  Request", asset)
			s.stats.updated++
			return nil
		}
		log.Infof("[%d/%d] Updating asset %d (serial: %s)", idx, total, existingAsset.ID, serial)
		resp, _, err := s.snipe.Assets.Update(existingAsset.ID, asset)
		if err != nil {
			return fmt.Errorf("updating asset %d: %w", existingAsset.ID, err)
		}
		if resp.Status == "error" {
			return fmt.Errorf("updating asset %d: %s", existingAsset.ID, resp.Message)
		}
		s.stats.updated++
	} else {
		// Create new asset
		asset := s.mapDeviceToAsset(device, "", "")
		if s.cfg.DryRun {
			log.Infof("[%d/%d] DRY RUN: Would create asset (serial: %s, model: %s %s)", idx, total, serial, device.Manufacturer, device.Model)
			s.debugJSON("  Request", asset)
			s.stats.created++
			return nil
		}
		log.Infof("[%d/%d] Creating asset (serial: %s, model: %s %s)", idx, total, serial, device.Manufacturer, device.Model)
		resp, _, err := s.snipe.Assets.Create(asset)
		if err != nil {
			return fmt.Errorf("creating asset: %w", err)
		}
		if resp.Status == "error" {
			return fmt.Errorf("creating asset: %s", resp.Message)
		}
		if resp.Payload.ID != 0 {
			assetID = resp.Payload.ID
		} else {
			// Look up the newly created asset by serial to get the ID
			created, _, err := s.snipe.Assets.GetAssetBySerial(serial)
			if err == nil && created != nil && created.Total > 0 {
				assetID = created.Rows[0].ID
			}
		}
		s.stats.created++
	}

	// Attach CODD PDF if available
	if assetID != 0 {
		coddURL := device.CODD
		if coddURL == "" {
			if ret, ok := s.returnBySerial[device.SerialNumber]; ok {
				coddURL = ret.CODD
			}
		}
		if coddURL != "" {
			if err := s.attachCODD(assetID, serial, coddURL); err != nil {
				log.Warnf("Failed to attach CODD for serial %s: %v", serial, err)
			}
		}
	}

	return nil
}

const (
	retrieverNotesStart = "--- BEGIN RETRIEVER DATA ---"
	retrieverNotesEnd   = "--- END RETRIEVER DATA ---"
)

func (s *syncer) mapDeviceToAsset(device retriever.WarehouseDevice, existingName, existingNotes string) snipeit.Asset {
	asset := snipeit.Asset{
		Serial:   device.SerialNumber,
		AssetTag: device.AssetTag,
	}

	// Preserve existing name; only set from model info if no name exists yet
	if existingName != "" {
		asset.CommonFields.Name = existingName
	} else if device.Manufacturer != "" && device.Model != "" {
		asset.CommonFields.Name = fmt.Sprintf("%s %s", device.Manufacturer, device.Model)
	} else if device.Model != "" {
		asset.CommonFields.Name = device.Model
	}

	// Map status — check returns/deployments for en-route overrides
	asset.StatusLabel = snipeit.StatusLabel{
		CommonFields: snipeit.CommonFields{ID: s.resolveStatus(device)},
	}

	// Set model ID — check model_mappings first (case-insensitive), then fall back to default
	modelID := s.cfg.DefaultModelID
	if len(s.cfg.ModelMappings) > 0 {
		deviceModel := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s %s", device.Manufacturer, device.Model)))
		for mapKey, mapID := range s.cfg.ModelMappings {
			if strings.ToLower(mapKey) == deviceModel {
				modelID = mapID
				break
			}
		}
	}
	if modelID != 0 {
		asset.Model = snipeit.Model{
			CommonFields: snipeit.CommonFields{ID: modelID},
		}
	}

	// Set location — devices in warehouse-related statuses go to Retriever Warehouse
	if s.cfg.WarehouseLocationID != 0 && isWarehouseStatus(device.Status) {
		asset.Location = snipeit.Location{
			CommonFields: snipeit.CommonFields{ID: s.cfg.WarehouseLocationID},
		}
	}

	// Map custom fields
	if len(s.cfg.CustomFields) > 0 {
		cf := make(map[string]string)
		for snipeField, retrieverField := range s.cfg.CustomFields {
			val := getRetrieverField(device, retrieverField)
			// Fallback: check device return for fields not on warehouse device
			if val == "" {
				if ret, ok := s.returnBySerial[device.SerialNumber]; ok {
					switch retrieverField {
					case "codd":
						val = ret.CODD
					case "legal_hold":
						if ret.LegalHold != nil {
							val = fmt.Sprintf("%d", *ret.LegalHold)
						}
					}
				}
			}
			if val != "" {
				cf[snipeField] = val
			}
		}
		if len(cf) > 0 {
			asset.CustomFields = cf
		}
	}

	// Build the Retriever data block
	var rNotes []string
	if device.Notes != "" {
		rNotes = append(rNotes, device.Notes)
	}
	if device.CurrentLocation != "" {
		rNotes = append(rNotes, fmt.Sprintf("Location: %s", device.CurrentLocation))
	}
	if device.RecipientSource != "" {
		rNotes = append(rNotes, fmt.Sprintf("Source: %s", device.RecipientSource))
	}

	// Add deployment info if available
	if deployment, ok := s.deploymentByDeviceID[device.ID]; ok {
		rNotes = append(rNotes, fmt.Sprintf("Deployment ID: %s", deployment.ID))
		if deployment.Shipment.TrackingNumber != "" {
			rNotes = append(rNotes, fmt.Sprintf("Deployment tracking: %s", deployment.Shipment.TrackingNumber))
		}
		if deployment.EmployeeInfo.Name != "" {
			rNotes = append(rNotes, fmt.Sprintf("Deployed to: %s", deployment.EmployeeInfo.Name))
		}
		if deployment.EmployeeInfo.Email != "" {
			rNotes = append(rNotes, fmt.Sprintf("Employee email: %s", deployment.EmployeeInfo.Email))
		}
		if deployment.Shipment.Status != "" {
			rNotes = append(rNotes, fmt.Sprintf("Deployment shipment status: %s", deployment.Shipment.Status))
		}
		if deployment.CreatedAt != "" {
			rNotes = append(rNotes, fmt.Sprintf("Deployment created: %s", deployment.CreatedAt))
		}
	}

	// Add return info if available
	if ret, ok := s.returnBySerial[device.SerialNumber]; ok {
		rNotes = append(rNotes, fmt.Sprintf("Return ID: %s", ret.ID))
		if ret.EmployeeInfo.Name != "" {
			rNotes = append(rNotes, fmt.Sprintf("Return from: %s", ret.EmployeeInfo.Name))
		}
		if len(ret.Shipments) > 0 {
			latest := ret.Shipments[len(ret.Shipments)-1]
			rNotes = append(rNotes, fmt.Sprintf("Return shipment status: %s", latest.Status))
			if latest.OutboundTracking != nil && *latest.OutboundTracking != "" {
				rNotes = append(rNotes, fmt.Sprintf("Return box tracking: %s", *latest.OutboundTracking))
			}
			if latest.ReturnTracking != nil && *latest.ReturnTracking != "" {
				rNotes = append(rNotes, fmt.Sprintf("Return device tracking: %s", *latest.ReturnTracking))
			}
		}
		if ret.CreatedAt != "" {
			rNotes = append(rNotes, fmt.Sprintf("Return created: %s", ret.CreatedAt))
		}
	}

	retrieverBlock := retrieverNotesStart + "\n" + strings.Join(rNotes, "\n") + "\n" + retrieverNotesEnd

	// Merge with existing notes: strip old Retriever block, keep user notes
	asset.CommonFields.Notes = mergeNotes(existingNotes, retrieverBlock)

	// Use serial number as asset tag if none set
	if asset.AssetTag == "" {
		asset.AssetTag = device.SerialNumber
	}

	return asset
}

// assetNeedsUpdate compares the desired asset state with the existing Snipe-IT
// asset and returns true if any tracked field differs. Custom fields are always
// considered changed when configured, since the GET response doesn't return
// them in the same format we write.
func assetNeedsUpdate(existing snipeit.Asset, desired snipeit.Asset) (bool, string) {
	if desired.CommonFields.Name != existing.CommonFields.Name {
		return true, fmt.Sprintf("name: %q -> %q", existing.CommonFields.Name, desired.CommonFields.Name)
	}
	if desired.AssetTag != existing.AssetTag {
		return true, fmt.Sprintf("asset_tag: %q -> %q", existing.AssetTag, desired.AssetTag)
	}
	if desired.StatusLabel.ID != 0 && desired.StatusLabel.ID != existing.StatusLabel.ID {
		return true, fmt.Sprintf("status_id: %d -> %d", existing.StatusLabel.ID, desired.StatusLabel.ID)
	}
	if desired.Model.ID != 0 && desired.Model.ID != existing.Model.ID {
		return true, fmt.Sprintf("model_id: %d -> %d", existing.Model.ID, desired.Model.ID)
	}
	if desired.Location.ID != 0 && desired.Location.ID != existing.Location.ID {
		return true, fmt.Sprintf("location_id: %d -> %d", existing.Location.ID, desired.Location.ID)
	}
	if normalizeNotes(desired.CommonFields.Notes) != normalizeNotes(existing.CommonFields.Notes) {
		return true, "notes changed"
	}
	for k, desiredVal := range desired.CustomFields {
		if existingVal, ok := existing.CustomFields[k]; !ok || existingVal != desiredVal {
			return true, fmt.Sprintf("custom field %s: %q -> %q", k, existingVal, desiredVal)
		}
	}
	return false, ""
}

// resolveStatus determines the Snipe-IT status ID by checking device returns
// and deployments for active shipment data, then falling back to the warehouse status.
func (s *syncer) resolveStatus(device retriever.WarehouseDevice) int {
	// 1. Check for an active device return — override to return_initiated/retrieval_initiated
	if ret, ok := s.returnBySerial[device.SerialNumber]; ok {
		if shipStatus := latestReturnShipmentStatus(ret); shipStatus != "" {
			switch shipStatus {
			case "box_in_transit", "box_delivered":
				// Return box sent/delivered — map as return_initiated
				if id, ok := s.cfg.StatusMappings["return_initiated"]; ok {
					return id
				}
			case "received_at_warehouse", "return_complete":
				// Already back — let warehouse status handle it
			case "deactivated":
				// Return cancelled — ignore
			default:
				// Unknown return shipment status — treat as return_initiated
				if id, ok := s.cfg.StatusMappings["return_initiated"]; ok {
					return id
				}
			}
		}
	}

	// 2. Check for an active deployment — override to deployment_initiated
	if dep, ok := s.deploymentByDeviceID[device.ID]; ok {
		switch dep.Shipment.Status {
		case "awaiting_shipment", "device_shipped":
			if id, ok := s.cfg.StatusMappings["deployment_initiated"]; ok {
				return id
			}
		case "deployed":
			// Already delivered
		case "deactivated":
			// Deployment cancelled
		}
	}

	// 3. Direct lookup from status_mappings
	if id, ok := s.cfg.StatusMappings[device.Status]; ok {
		return id
	}
	return s.cfg.DefaultStatusID
}

// latestReturnShipmentStatus returns the status of the most recent shipment
// in a device return, or empty string if there are no shipments.
func latestReturnShipmentStatus(ret retriever.DeviceReturn) string {
	if len(ret.Shipments) == 0 {
		return ""
	}
	// The last shipment in the array is the most recent
	return ret.Shipments[len(ret.Shipments)-1].Status
}

// googleDriveFileIDRegexp extracts the file ID from a Google Drive sharing URL.
var googleDriveFileIDRegexp = regexp.MustCompile(`/file/d/([^/]+)`)

// attachCODD downloads the CODD PDF from the given URL and attaches it to the Snipe-IT asset.
func (s *syncer) attachCODD(assetID int, serial, coddURL string) error {
	// Convert Google Drive sharing URL to direct download URL
	downloadURL := coddURL
	if matches := googleDriveFileIDRegexp.FindStringSubmatch(coddURL); len(matches) == 2 {
		downloadURL = fmt.Sprintf("https://drive.google.com/uc?export=download&id=%s", matches[1])
	}

	// Download the PDF
	httpResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading CODD: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading CODD: HTTP %d", httpResp.StatusCode)
	}

	pdfData, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("reading CODD response: %w", err)
	}

	// Build multipart form for Snipe-IT file upload
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", fmt.Sprintf("CODD_%s.pdf", serial))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(pdfData); err != nil {
		return fmt.Errorf("writing PDF data: %w", err)
	}

	// Add notes field
	if err := writer.WriteField("notes", fmt.Sprintf("Certificate of Data Destruction — %s", coddURL)); err != nil {
		return fmt.Errorf("writing notes field: %w", err)
	}
	writer.Close()

	// Upload to Snipe-IT
	uploadPath := fmt.Sprintf("api/v1/hardware/%d/files", assetID)
	req, err := s.snipe.NewRequest(http.MethodPost, uploadPath, nil)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Body = io.NopCloser(&buf)
	req.ContentLength = int64(buf.Len())

	var uploadResp struct {
		Status   string `json:"status"`
		Messages string `json:"messages"`
	}
	if _, err := s.snipe.Do(req, &uploadResp); err != nil {
		return fmt.Errorf("uploading CODD: %w", err)
	}
	if uploadResp.Status != "success" {
		return fmt.Errorf("uploading CODD: status=%s messages=%s", uploadResp.Status, uploadResp.Messages)
	}

	log.Infof("  Attached CODD PDF for serial %s (asset %d)", serial, assetID)
	return nil
}

// isWarehouseStatus returns true if the Retriever status indicates the device
// is physically at the Retriever warehouse.
func isWarehouseStatus(status string) bool {
	switch status {
	case "device_received", "ready_for_deployment", "provisioning",
		"in_repair", "requires_service", "administrative_hold",
		"legal_hold", "input_required":
		return true
	}
	return false
}

func (s *syncer) debugJSON(label string, v any) {
	if !log.IsLevelEnabled(log.DebugLevel) {
		return
	}
	data, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		log.Debugf("Failed to marshal %s: %v", label, err)
		return
	}
	log.Debugf("%s:\n  %s", label, string(data))
}

// snipeLogger implements snipeit.Logger to log HTTP requests and responses
// when --debug is enabled.
type snipeLogger struct{}

func (l *snipeLogger) LogRequest(method, url string, body []byte) {
	if len(body) > 0 {
		log.Debugf("-> %s %s\n  %s", method, url, string(body))
	} else {
		log.Debugf("-> %s %s", method, url)
	}
}

func (l *snipeLogger) LogResponse(method, url string, statusCode int, body []byte) {
	if len(body) > 0 {
		log.Debugf("<- %d %s\n  %s", statusCode, url, string(body))
	} else {
		log.Debugf("<- %d %s", statusCode, url)
	}
}

// mergeNotes strips any existing Retriever data block from the existing notes
// and appends the new block. User-written notes outside the markers are preserved.
// normalizeNotes strips differences that Snipe-IT introduces on storage
// (HTML entity encoding, \r\n line endings) so that notes comparison is stable.
func normalizeNotes(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return s
}

func mergeNotes(existing, newBlock string) string {
	// Strip old Retriever block if present
	cleaned := existing
	if startIdx := strings.Index(existing, retrieverNotesStart); startIdx != -1 {
		endIdx := strings.Index(existing, retrieverNotesEnd)
		if endIdx != -1 {
			endIdx += len(retrieverNotesEnd)
			// Trim any surrounding whitespace/newlines around the old block
			before := strings.TrimRight(existing[:startIdx], "\n ")
			after := strings.TrimLeft(existing[endIdx:], "\n ")
			if before != "" && after != "" {
				cleaned = before + "\n" + after
			} else {
				cleaned = before + after
			}
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return newBlock
	}
	return cleaned + "\n\n" + newBlock
}

// getRetrieverField extracts a value from a WarehouseDevice by field name,
// applying unit conversions where needed.
func getRetrieverField(device retriever.WarehouseDevice, field string) string {
	switch field {
	case "serial_number":
		return device.SerialNumber
	case "manufacturer":
		return device.Manufacturer
	case "model":
		return device.Model
	case "device_series":
		return device.DeviceSeries
	case "release_year":
		return device.ReleaseYear
	case "ram":
		return convertToMB(device.RAM)
	case "disk", "storage":
		return convertToGB(device.Disk)
	case "processor":
		return device.Processor
	case "screen_size":
		return device.GetScreenSize()
	case "operating_system":
		return device.OperatingSystem
	case "os_version":
		return device.OSVersion
	case "status":
		return device.Status
	case "current_location":
		return device.CurrentLocation
	case "asset_tag":
		return device.AssetTag
	case "rating":
		return device.GetRating()
	case "recipient_source":
		return device.RecipientSource
	case "notes":
		return device.Notes
	case "id":
		return device.ID
	case "has_charger":
		if device.HasCharger {
			return "1"
		}
		return "0"
	case "codd":
		return device.CODD
	case "legal_hold":
		// Value comes from DeviceReturn.LegalHold, handled via returnBySerial fallback
		return ""
	default:
		return ""
	}
}

// convertToMB parses a value like "16 GB" and returns megabytes as a string ("16384").
func convertToMB(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}
	upper := strings.ToUpper(val)
	if strings.HasSuffix(upper, " GB") {
		numStr := strings.TrimSpace(val[:len(val)-3])
		n, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return val
		}
		return strconv.Itoa(int(n * 1024))
	}
	if strings.HasSuffix(upper, " MB") {
		numStr := strings.TrimSpace(val[:len(val)-3])
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return val
		}
		return strconv.Itoa(n)
	}
	return val
}

// convertToGB parses a value like "256 GB" or "1 TB" and returns gigabytes as a string.
func convertToGB(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}
	upper := strings.ToUpper(val)
	if strings.HasSuffix(upper, " TB") {
		numStr := strings.TrimSpace(val[:len(val)-3])
		n, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return val
		}
		return strconv.Itoa(int(n * 1024))
	}
	if strings.HasSuffix(upper, " GB") {
		numStr := strings.TrimSpace(val[:len(val)-3])
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return val
		}
		return strconv.Itoa(n)
	}
	return val
}
