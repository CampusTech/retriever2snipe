package retriever

// WarehouseDevice represents a device in the Retriever warehouse (inventory).
type WarehouseDevice struct {
	ID                string  `json:"id"`
	SerialNumber      string  `json:"serial_number"`
	Manufacturer      string  `json:"manufacturer"`
	Model             string  `json:"model"`
	DeviceSeries      string  `json:"device_series"`
	ReleaseYear       string  `json:"release_year"`
	Rating            *string `json:"rating"`
	RAM               string  `json:"ram"`
	Disk              string  `json:"disk"`
	Processor         string  `json:"processor"`
	ScreenSize        *string `json:"screen_size"`
	OperatingSystem   string  `json:"operating_system"`
	OSVersion         string  `json:"os_version"`
	RecipientSource   string  `json:"recipient_source"`
	Status            string  `json:"status"`
	CurrentLocation   string  `json:"current_location"`
	AssetTag          string  `json:"asset_tag"`
	HasCharger        bool    `json:"has_charger"`
	CustomerName      *string `json:"customer_name"`
	InitialDeployment *string `json:"initial_deployment"`
	Notes             string  `json:"notes"`
	CODD              string  `json:"codd"`
}

// GetRating returns the rating value or empty string if nil.
func (d WarehouseDevice) GetRating() string {
	if d.Rating != nil {
		return *d.Rating
	}
	return ""
}

// GetScreenSize returns the screen size value or empty string if nil.
func (d WarehouseDevice) GetScreenSize() string {
	if d.ScreenSize != nil {
		return *d.ScreenSize
	}
	return ""
}

// Deployment represents a device deployment order.
type Deployment struct {
	ID             string             `json:"id"`
	DeviceID       string             `json:"device_id"`
	PurchaserEmail string             `json:"purchaser_email"`
	CreatedViaAPI  bool               `json:"created_via_api"`
	Notes          *string            `json:"notes"`
	ShippingSpeed  *string            `json:"shipping_speed"`
	CreatedAt      string             `json:"created_at"`
	EmployeeInfo   EmployeeInfo       `json:"employee_info"`
	Shipment       DeploymentShipment `json:"shipment"`
	DisplayName    string             `json:"display_name"`
	TicketID       *string            `json:"ticket_id"`
}

// DeploymentShipment contains shipping details for a deployment.
type DeploymentShipment struct {
	Status          string  `json:"status"`
	TrackingNumber  string  `json:"tracking_number"`
	ReturnTracking  string  `json:"return_tracking"`
	Carrier         string  `json:"carrier"`
	DeviceShippedAt *string `json:"device_shipped_at"`
	DeviceDelivered *string `json:"device_delivered_at"`
}

// EmployeeInfo contains employee/recipient information.
type EmployeeInfo struct {
	Email          string  `json:"email"`
	Name           string  `json:"name"`
	AddressLine1   string  `json:"address_line_1"`
	AddressLine2   *string `json:"address_line_2"`
	AddressCity    string  `json:"address_city"`
	AddressState   *string `json:"address_state"`
	AddressZip     string  `json:"address_zip"`
	AddressCountry string  `json:"address_country"`
}

// DeviceReturn represents a device return order.
type DeviceReturn struct {
	ID              string           `json:"id"`
	URL             string           `json:"url"`
	SerialNumber    *string          `json:"serial_number"`
	TicketID        *string          `json:"ticket_id"`
	PurchaserEmail  string           `json:"purchaser_email"`
	CreatedViaAPI   bool             `json:"created_via_api"`
	CreatedAt       string           `json:"created_at"`
	CODD            string           `json:"codd"`
	Note1           *string          `json:"note_1"`
	Note2           *string          `json:"note_2"`
	EmployeeInfo    EmployeeInfo     `json:"employee_info"`
	Shipments       []DeviceShipment `json:"shipments"`
	LegalHold       *int             `json:"legal_hold"`
	RequestCharger  bool             `json:"request_charger"`
	RequestCellPhone bool            `json:"request_cell_phone"`
	Device          string           `json:"device"`
}

// DeviceShipment contains shipping details for a device return.
type DeviceShipment struct {
	Status           string  `json:"status"`
	OutboundTracking *string `json:"outbound_tracking"`
	OutboundCarrier  *string `json:"outbound_carrier"`
	ReturnTracking   *string `json:"return_tracking"`
	ReturnCarrier    *string `json:"return_carrier"`
	BoxShippedAt     *string `json:"box_shipped_at"`
	BoxDeliveredAt   *string `json:"box_delivered_at"`
	DeviceShippedAt  *string `json:"device_shipped_at"`
	DeviceDeliveredAt *string `json:"device_delivered_at"`
}
