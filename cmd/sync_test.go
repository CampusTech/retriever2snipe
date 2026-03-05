package cmd

import (
	"testing"

	"github.com/CampusTech/retriever2snipe/retriever"
)

func TestConvertToMB(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"16 GB", "16384"},
		{"8 GB", "8192"},
		{"4 GB", "4096"},
		{"512 MB", "512"},
		{"1024 MB", "1024"},
		{"0.5 GB", "512"},
		{"unknown", "unknown"},
		{"  16 GB  ", "16384"},   // trimmed
		{"16 gb", "16384"},       // case insensitive
		{"bad GB", "bad GB"},     // unparseable number
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := convertToMB(tt.input); got != tt.want {
				t.Errorf("convertToMB(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertToGB(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"256 GB", "256"},
		{"512 GB", "512"},
		{"1 TB", "1024"},
		{"2 TB", "2048"},
		{"0.5 TB", "512"},
		{"unknown", "unknown"},
		{"  1 TB  ", "1024"},    // trimmed
		{"1 tb", "1024"},        // case insensitive
		{"bad GB", "bad GB"},    // unparseable number
		{"bad TB", "bad TB"},    // unparseable number
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := convertToGB(tt.input); got != tt.want {
				t.Errorf("convertToGB(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsWarehouseStatus(t *testing.T) {
	warehouseStatuses := []string{
		"device_received", "ready_for_deployment", "provisioning",
		"in_repair", "requires_service", "administrative_hold",
		"legal_hold", "input_required",
	}
	for _, s := range warehouseStatuses {
		if !isWarehouseStatus(s) {
			t.Errorf("isWarehouseStatus(%q) = false, want true", s)
		}
	}

	nonWarehouseStatuses := []string{
		"deployed", "disposed", "return_initiated", "lost_in_transit",
		"deployment_initiated", "to_be_retired", "returned_to_vendor",
		"", "unknown",
	}
	for _, s := range nonWarehouseStatuses {
		if isWarehouseStatus(s) {
			t.Errorf("isWarehouseStatus(%q) = true, want false", s)
		}
	}
}

func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }

func TestGetRetrieverField(t *testing.T) {
	rating := "B+"
	screenSize := "15.6"
	device := retriever.WarehouseDevice{
		ID:              "dev-1",
		SerialNumber:    "SN123",
		Manufacturer:    "Dell",
		Model:           "Latitude 5520",
		DeviceSeries:    "Latitude",
		ReleaseYear:     "2024",
		Rating:          &rating,
		RAM:             "16 GB",
		Disk:            "512 GB",
		Processor:       "Intel i7",
		ScreenSize:      &screenSize,
		OperatingSystem: "Windows",
		OSVersion:       "11",
		Status:          "deployed",
		CurrentLocation: "Warehouse A",
		AssetTag:        "TAG-001",
		HasCharger:      true,
		RecipientSource: "IT Dept",
		Notes:           "Test device",
		CODD:            "https://example.com/codd.pdf",
	}

	tests := []struct {
		field string
		want  string
	}{
		{"serial_number", "SN123"},
		{"manufacturer", "Dell"},
		{"model", "Latitude 5520"},
		{"device_series", "Latitude"},
		{"release_year", "2024"},
		{"ram", "16384"},           // converted to MB
		{"disk", "512"},            // converted to GB
		{"storage", "512"},         // alias for disk
		{"processor", "Intel i7"},
		{"screen_size", "15.6"},
		{"operating_system", "Windows"},
		{"os_version", "11"},
		{"status", "deployed"},
		{"current_location", "Warehouse A"},
		{"asset_tag", "TAG-001"},
		{"rating", "B+"},
		{"recipient_source", "IT Dept"},
		{"notes", "Test device"},
		{"id", "dev-1"},
		{"has_charger", "1"},
		{"codd", "https://example.com/codd.pdf"},
		{"legal_hold", ""},          // always empty from device, filled from return
		{"unknown_field", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if got := getRetrieverField(device, tt.field); got != tt.want {
				t.Errorf("getRetrieverField(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}

	// Test has_charger = false
	device.HasCharger = false
	if got := getRetrieverField(device, "has_charger"); got != "0" {
		t.Errorf("has_charger false = %q, want %q", got, "0")
	}

	// Test nil optional fields
	device.Rating = nil
	device.ScreenSize = nil
	if got := getRetrieverField(device, "rating"); got != "" {
		t.Errorf("nil rating = %q, want empty", got)
	}
	if got := getRetrieverField(device, "screen_size"); got != "" {
		t.Errorf("nil screen_size = %q, want empty", got)
	}
}

func TestMergeNotes(t *testing.T) {
	start := "--- BEGIN RETRIEVER DATA ---"
	end := "--- END RETRIEVER DATA ---"

	newBlock := start + "\nNew data\nLast synced: now\n" + end

	tests := []struct {
		name     string
		existing string
		want     string
	}{
		{
			"empty existing",
			"",
			newBlock,
		},
		{
			"no existing block",
			"User notes here",
			"User notes here\n\n" + newBlock,
		},
		{
			"replace existing block",
			start + "\nOld data\n" + end,
			newBlock,
		},
		{
			"preserve user notes before block",
			"My notes\n\n" + start + "\nOld data\n" + end,
			"My notes\n\n" + newBlock,
		},
		{
			"preserve user notes after block",
			start + "\nOld data\n" + end + "\n\nUser notes after",
			"User notes after\n\n" + newBlock,
		},
		{
			"preserve user notes on both sides",
			"Before\n\n" + start + "\nOld data\n" + end + "\n\nAfter",
			"Before\nAfter\n\n" + newBlock,
		},
		{
			"whitespace only existing",
			"   \n\n   ",
			newBlock,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeNotes(tt.existing, newBlock)
			if got != tt.want {
				t.Errorf("mergeNotes():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestGoogleDriveFileIDRegexp(t *testing.T) {
	tests := []struct {
		url  string
		want string // expected file ID, empty if no match
	}{
		{"https://drive.google.com/file/d/1aBcDeFgHiJkLmNoPqRsTuVwXyZ/view?usp=sharing", "1aBcDeFgHiJkLmNoPqRsTuVwXyZ"},
		{"https://drive.google.com/file/d/abc123/view", "abc123"},
		{"https://example.com/not-a-drive-link", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			matches := googleDriveFileIDRegexp.FindStringSubmatch(tt.url)
			if tt.want == "" {
				if len(matches) >= 2 {
					t.Errorf("expected no match, got %q", matches[1])
				}
			} else {
				if len(matches) < 2 {
					t.Fatalf("expected match %q, got no match", tt.want)
				}
				if matches[1] != tt.want {
					t.Errorf("file ID = %q, want %q", matches[1], tt.want)
				}
			}
		})
	}
}

func TestResolveStatus(t *testing.T) {
	cfg := Config{
		DefaultStatusID: 99,
		StatusMappings: map[string]int{
			"deployed":             1,
			"device_received":      2,
			"return_initiated":     3,
			"deployment_initiated": 4,
			"ready_for_deployment": 5,
		},
	}

	t.Run("direct status mapping", func(t *testing.T) {
		s := &syncer{
			cfg:                   cfg,
			deploymentByDeviceID:  make(map[string]retriever.Deployment),
			returnBySerial:        make(map[string]retriever.DeviceReturn),
		}
		device := retriever.WarehouseDevice{Status: "deployed"}
		if got := s.resolveStatus(device); got != 1 {
			t.Errorf("resolveStatus(deployed) = %d, want 1", got)
		}
	})

	t.Run("default status for unknown", func(t *testing.T) {
		s := &syncer{
			cfg:                   cfg,
			deploymentByDeviceID:  make(map[string]retriever.Deployment),
			returnBySerial:        make(map[string]retriever.DeviceReturn),
		}
		device := retriever.WarehouseDevice{Status: "unknown_status"}
		if got := s.resolveStatus(device); got != 99 {
			t.Errorf("resolveStatus(unknown) = %d, want 99 (default)", got)
		}
	})

	t.Run("return override with box_in_transit", func(t *testing.T) {
		s := &syncer{
			cfg:                  cfg,
			deploymentByDeviceID: make(map[string]retriever.Deployment),
			returnBySerial: map[string]retriever.DeviceReturn{
				"SN1": {
					Shipments: []retriever.DeviceShipment{
						{Status: "box_in_transit"},
					},
				},
			},
		}
		device := retriever.WarehouseDevice{SerialNumber: "SN1", Status: "device_received"}
		if got := s.resolveStatus(device); got != 3 {
			t.Errorf("resolveStatus with active return = %d, want 3 (return_initiated)", got)
		}
	})

	t.Run("deployment override with device_shipped", func(t *testing.T) {
		s := &syncer{
			cfg: cfg,
			deploymentByDeviceID: map[string]retriever.Deployment{
				"dev-1": {
					Shipment: retriever.DeploymentShipment{Status: "device_shipped"},
				},
			},
			returnBySerial: make(map[string]retriever.DeviceReturn),
		}
		device := retriever.WarehouseDevice{ID: "dev-1", Status: "device_received"}
		if got := s.resolveStatus(device); got != 4 {
			t.Errorf("resolveStatus with active deployment = %d, want 4 (deployment_initiated)", got)
		}
	})
}

func TestLatestReturnShipmentStatus(t *testing.T) {
	tests := []struct {
		name      string
		shipments []retriever.DeviceShipment
		want      string
	}{
		{"no shipments", nil, ""},
		{"empty shipments", []retriever.DeviceShipment{}, ""},
		{"single shipment", []retriever.DeviceShipment{{Status: "box_in_transit"}}, "box_in_transit"},
		{"multiple shipments returns last", []retriever.DeviceShipment{
			{Status: "box_in_transit"},
			{Status: "device_shipped"},
		}, "device_shipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ret := retriever.DeviceReturn{Shipments: tt.shipments}
			if got := latestReturnShipmentStatus(ret); got != tt.want {
				t.Errorf("latestReturnShipmentStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
