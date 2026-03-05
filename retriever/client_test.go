package retriever

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(server *httptest.Server) *Client {
	return &Client{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: server.Client(),
		limiter:    newRateLimiter(1000, time.Second), // high limit for tests
	}
}

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want %q", got, "application/json")
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := newTestClient(server)
	var result map[string]string
	err := client.get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %q, want %q", result["status"], "ok")
	}
}

func TestClientGet429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newTestClient(server)
	var result map[string]string
	err := client.get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if got := err.Error(); got != "rate limited (HTTP 429) — reduce request frequency or wait" {
		t.Errorf("error = %q", got)
	}
}

func TestClientGetHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := newTestClient(server)
	var result map[string]string
	err := client.get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if got := err.Error(); got != "HTTP 500: internal error" {
		t.Errorf("error = %q", got)
	}
}

func TestClientGetContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	client := newTestClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var result map[string]string
	err := client.get(ctx, "/test", &result)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPaginateSinglePage(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(paginatedResponse[item]{
			Results: []item{{ID: "a"}, {ID: "b"}, {ID: "c"}},
			Next:    nil,
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	items, err := paginate[item](context.Background(), client, "/api/v2/items/")
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if items[2].ID != "c" {
		t.Errorf("items[2].ID = %q, want %q", items[2].ID, "c")
	}
}

func TestPaginateMultiPage(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}

	callCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch callCount {
		case 1:
			next := serverURL + "/api/v2/items/?page=2"
			json.NewEncoder(w).Encode(paginatedResponse[item]{
				Results: []item{{ID: "1"}, {ID: "2"}},
				Next:    &next,
			})
		case 2:
			json.NewEncoder(w).Encode(paginatedResponse[item]{
				Results: []item{{ID: "3"}},
				Next:    nil,
			})
		default:
			t.Errorf("unexpected call %d", callCount)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := newTestClient(server)
	items, err := paginate[item](context.Background(), client, "/api/v2/items/")
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if items[0].ID != "1" || items[2].ID != "3" {
		t.Errorf("items = %v", items)
	}
}

func TestExtractPath(t *testing.T) {
	tests := []struct {
		name        string
		absoluteURL string
		baseURL     string
		want        string
	}{
		{"strips base", "https://api.example.com/api/v2/items/?page=2", "https://api.example.com", "/api/v2/items/?page=2"},
		{"no match", "https://other.com/path", "https://api.example.com", "https://other.com/path"},
		{"empty base", "/api/v2/items/", "", "/api/v2/items/"},
		{"exact match returns input", "https://api.example.com", "https://api.example.com", "https://api.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractPath(tt.absoluteURL, tt.baseURL); got != tt.want {
				t.Errorf("extractPath(%q, %q) = %q, want %q", tt.absoluteURL, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestListAllWarehouseDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/warehouse/" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/api/v2/warehouse/")
		}
		json.NewEncoder(w).Encode(paginatedResponse[WarehouseDevice]{
			Results: []WarehouseDevice{
				{ID: "d1", SerialNumber: "SN1", Manufacturer: "Apple"},
				{ID: "d2", SerialNumber: "SN2", Manufacturer: "Dell"},
			},
			Next: nil,
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	devices, err := client.ListAllWarehouseDevices(context.Background())
	if err != nil {
		t.Fatalf("ListAllWarehouseDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("len = %d, want 2", len(devices))
	}
	if devices[0].Manufacturer != "Apple" {
		t.Errorf("devices[0].Manufacturer = %q, want %q", devices[0].Manufacturer, "Apple")
	}
}

func TestGetDeviceReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/device_returns/ret-42/" {
			t.Errorf("path = %q, want /api/v2/device_returns/ret-42/", r.URL.Path)
		}
		json.NewEncoder(w).Encode(DeviceReturn{
			ID:   "ret-42",
			CODD: "https://drive.google.com/file/d/abc/view",
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	ret, err := client.GetDeviceReturn(context.Background(), "ret-42")
	if err != nil {
		t.Fatalf("GetDeviceReturn: %v", err)
	}
	if ret.ID != "ret-42" {
		t.Errorf("ID = %q, want %q", ret.ID, "ret-42")
	}
	if ret.CODD != "https://drive.google.com/file/d/abc/view" {
		t.Errorf("CODD = %q", ret.CODD)
	}
}
