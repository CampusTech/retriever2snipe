package retriever

import (
	"encoding/json"
	"testing"
)

func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }

func TestGetRating(t *testing.T) {
	tests := []struct {
		name   string
		rating *string
		want   string
	}{
		{"nil rating", nil, ""},
		{"empty rating", stringPtr(""), ""},
		{"A rating", stringPtr("A"), "A"},
		{"B+ rating", stringPtr("B+"), "B+"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := WarehouseDevice{Rating: tt.rating}
			if got := d.GetRating(); got != tt.want {
				t.Errorf("GetRating() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetScreenSize(t *testing.T) {
	tests := []struct {
		name       string
		screenSize *string
		want       string
	}{
		{"nil screen size", nil, ""},
		{"empty screen size", stringPtr(""), ""},
		{"13 inch", stringPtr("13.3"), "13.3"},
		{"15 inch", stringPtr("15.6"), "15.6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := WarehouseDevice{ScreenSize: tt.screenSize}
			if got := d.GetScreenSize(); got != tt.want {
				t.Errorf("GetScreenSize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWarehouseDeviceJSON(t *testing.T) {
	data := `{
		"id": "dev-123",
		"serial_number": "ABC123",
		"manufacturer": "Apple",
		"model": "MacBook Pro",
		"ram": "16 GB",
		"disk": "512 GB",
		"has_charger": true,
		"status": "deployed",
		"rating": "A",
		"screen_size": "14.2",
		"codd": "https://example.com/codd.pdf"
	}`

	var device WarehouseDevice
	if err := json.Unmarshal([]byte(data), &device); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if device.ID != "dev-123" {
		t.Errorf("ID = %q, want %q", device.ID, "dev-123")
	}
	if device.SerialNumber != "ABC123" {
		t.Errorf("SerialNumber = %q, want %q", device.SerialNumber, "ABC123")
	}
	if device.HasCharger != true {
		t.Error("HasCharger = false, want true")
	}
	if device.GetRating() != "A" {
		t.Errorf("GetRating() = %q, want %q", device.GetRating(), "A")
	}
	if device.GetScreenSize() != "14.2" {
		t.Errorf("GetScreenSize() = %q, want %q", device.GetScreenSize(), "14.2")
	}
}

func TestDeploymentJSON(t *testing.T) {
	data := `{
		"id": "dep-1",
		"device_id": "dev-1",
		"purchaser_email": "buyer@example.com",
		"created_via_api": true,
		"created_at": "2026-01-15",
		"employee_info": {
			"name": "Alice",
			"email": "alice@example.com",
			"address_line_1": "123 Main St",
			"address_city": "Springfield",
			"address_zip": "12345",
			"address_country": "US"
		},
		"shipment": {
			"status": "device_shipped",
			"tracking_number": "1Z999AA10123456784",
			"carrier": "UPS"
		}
	}`

	var dep Deployment
	if err := json.Unmarshal([]byte(data), &dep); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if dep.DeviceID != "dev-1" {
		t.Errorf("DeviceID = %q, want %q", dep.DeviceID, "dev-1")
	}
	if dep.EmployeeInfo.Name != "Alice" {
		t.Errorf("EmployeeInfo.Name = %q, want %q", dep.EmployeeInfo.Name, "Alice")
	}
	if dep.Shipment.TrackingNumber != "1Z999AA10123456784" {
		t.Errorf("Shipment.TrackingNumber = %q, want %q", dep.Shipment.TrackingNumber, "1Z999AA10123456784")
	}
}

func TestDeviceReturnJSON(t *testing.T) {
	data := `{
		"id": "ret-1",
		"url": "https://example.com/ret-1",
		"serial_number": "XYZ789",
		"purchaser_email": "buyer@example.com",
		"codd": "https://drive.google.com/file/d/abc123/view",
		"legal_hold": 30,
		"request_charger": true,
		"created_at": "2026-02-01",
		"employee_info": {
			"name": "Bob",
			"email": "bob@example.com",
			"address_line_1": "456 Oak Ave",
			"address_city": "Portland",
			"address_zip": "97201",
			"address_country": "US"
		},
		"shipments": [
			{
				"status": "box_in_transit",
				"outbound_tracking": "TRK001"
			},
			{
				"status": "device_shipped",
				"return_tracking": "TRK002"
			}
		]
	}`

	var ret DeviceReturn
	if err := json.Unmarshal([]byte(data), &ret); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if ret.SerialNumber == nil || *ret.SerialNumber != "XYZ789" {
		t.Errorf("SerialNumber = %v, want %q", ret.SerialNumber, "XYZ789")
	}
	if ret.LegalHold == nil || *ret.LegalHold != 30 {
		t.Errorf("LegalHold = %v, want 30", ret.LegalHold)
	}
	if len(ret.Shipments) != 2 {
		t.Fatalf("len(Shipments) = %d, want 2", len(ret.Shipments))
	}
	if ret.Shipments[1].Status != "device_shipped" {
		t.Errorf("Shipments[1].Status = %q, want %q", ret.Shipments[1].Status, "device_shipped")
	}
	if ret.Shipments[1].ReturnTracking == nil || *ret.Shipments[1].ReturnTracking != "TRK002" {
		t.Errorf("Shipments[1].ReturnTracking = %v, want %q", ret.Shipments[1].ReturnTracking, "TRK002")
	}
}

func TestDeviceReturnNilOptionals(t *testing.T) {
	data := `{
		"id": "ret-2",
		"url": "",
		"purchaser_email": "",
		"codd": "",
		"created_at": "",
		"employee_info": {"name":"","email":"","address_line_1":"","address_city":"","address_zip":"","address_country":""},
		"shipments": []
	}`

	var ret DeviceReturn
	if err := json.Unmarshal([]byte(data), &ret); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if ret.SerialNumber != nil {
		t.Errorf("SerialNumber = %v, want nil", ret.SerialNumber)
	}
	if ret.LegalHold != nil {
		t.Errorf("LegalHold = %v, want nil", ret.LegalHold)
	}
	if ret.TicketID != nil {
		t.Errorf("TicketID = %v, want nil", ret.TicketID)
	}
}
