package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/CampusTech/retriever2snipe/retriever"
)

// slackBlock represents a single block in a Slack Block Kit message.
type slackBlock struct {
	Type     string          `json:"type"`
	Text     *slackText      `json:"text,omitempty"`
	Fields   []slackText     `json:"fields,omitempty"`
	Elements []slackElement  `json:"elements,omitempty"`
	ImageURL string          `json:"image_url,omitempty"`
	AltText  string          `json:"alt_text,omitempty"`
}

// slackText represents a text object in Slack Block Kit.
type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// slackElement represents an element in a context block.
type slackElement struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// slackMessage represents a Slack webhook message with Block Kit blocks.
type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

// statusDisplayName returns the human-readable label for a Retriever status key.
func statusDisplayName(status string) string {
	for _, s := range AllRetrieverStatuses {
		if s.Key == status {
			return s.Label
		}
	}
	return strings.ReplaceAll(status, "_", " ")
}

// slackNotifyNewAsset sends a Slack notification for a newly created Snipe-IT asset.
func slackNotifyNewAsset(webhookURL string, device retriever.WarehouseDevice, assetID int, snipeURL string) error {
	deviceName := strings.TrimSpace(fmt.Sprintf("%s %s", device.Manufacturer, device.Model))
	if deviceName == "" {
		deviceName = "Unknown Device"
	}

	assetURL := fmt.Sprintf("%s/hardware/%d", strings.TrimRight(snipeURL, "/"), assetID)
	statusLabel := statusDisplayName(device.Status)

	// Build fields for the primary info section
	primaryFields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Serial Number*\n`%s`", device.SerialNumber)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Device*\n%s", deviceName)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Status*\n%s", statusLabel)},
	}
	if device.AssetTag != "" {
		primaryFields = append(primaryFields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Asset Tag*\n`%s`", device.AssetTag)})
	}

	// Build fields for specs section (only include non-empty values)
	var specFields []slackText
	if device.RAM != "" {
		specFields = append(specFields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*RAM*\n%s", device.RAM)})
	}
	if device.Disk != "" {
		specFields = append(specFields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Storage*\n%s", device.Disk)})
	}
	if device.Processor != "" {
		specFields = append(specFields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Processor*\n%s", device.Processor)})
	}
	if device.CurrentLocation != "" {
		specFields = append(specFields, slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Location*\n%s", device.CurrentLocation)})
	}

	blocks := []slackBlock{
		{
			Type: "header",
			Text: &slackText{Type: "plain_text", Text: "New Asset Added to Snipe-IT"},
		},
		{
			Type:   "section",
			Fields: primaryFields,
		},
	}

	if len(specFields) > 0 {
		blocks = append(blocks, slackBlock{
			Type:   "section",
			Fields: specFields,
		})
	}

	blocks = append(blocks,
		slackBlock{Type: "divider"},
		slackBlock{
			Type: "context",
			Elements: []slackElement{
				{Type: "mrkdwn", Text: fmt.Sprintf("<%s|View in Snipe-IT>  |  Retriever ID: `%s`  |  %s", assetURL, device.ID, time.Now().Format("Jan 2, 2006 3:04 PM"))},
			},
		},
	)

	msg := slackMessage{Blocks: blocks}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling slack message: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("posting to slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %s", resp.Status)
	}

	return nil
}
