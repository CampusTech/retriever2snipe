// Package retriever provides a client for the Retriever v2 API.
package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Retriever v2 API client with built-in rate limiting.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	limiter    *rateLimiter
}

// NewClient creates a new Retriever API client.
// Rate limited to 50 req/min (under the 60/min limit) to be safe.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		limiter: newRateLimiter(50, time.Minute),
	}
}

func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate limited (HTTP 429) — reduce request frequency or wait")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// ListAllWarehouseDevices fetches all warehouse devices, handling pagination.
func (c *Client) ListAllWarehouseDevices(ctx context.Context) ([]WarehouseDevice, error) {
	return paginate[WarehouseDevice](ctx, c, "/api/v2/warehouse/")
}

// ListAllDeployments fetches all deployments, handling pagination.
func (c *Client) ListAllDeployments(ctx context.Context) ([]Deployment, error) {
	return paginate[Deployment](ctx, c, "/api/v2/deployments/")
}

// ListAllDeviceReturns fetches all device returns, handling pagination.
func (c *Client) ListAllDeviceReturns(ctx context.Context) ([]DeviceReturn, error) {
	return paginate[DeviceReturn](ctx, c, "/api/v2/device_returns/")
}

// GetDeviceReturn fetches a single device return by ID (detail endpoint has CODD URL).
func (c *Client) GetDeviceReturn(ctx context.Context, id string) (*DeviceReturn, error) {
	var result DeviceReturn
	if err := c.get(ctx, fmt.Sprintf("/api/v2/device_returns/%s/", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// paginatedResponse is the generic paginated response from Retriever.
type paginatedResponse[T any] struct {
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []T     `json:"results"`
}

func paginate[T any](ctx context.Context, c *Client, initialPath string) ([]T, error) {
	var all []T
	path := initialPath
	page := 1

	for {
		var resp paginatedResponse[T]
		if err := c.get(ctx, path, &resp); err != nil {
			return all, fmt.Errorf("page %d: %w", page, err)
		}

		all = append(all, resp.Results...)

		if resp.Next == nil || *resp.Next == "" {
			break
		}

		// The next URL is absolute; extract the path portion
		path = extractPath(*resp.Next, c.baseURL)
		page++
	}

	return all, nil
}

// extractPath strips the base URL from an absolute URL to get the relative path.
func extractPath(absoluteURL, baseURL string) string {
	if len(absoluteURL) > len(baseURL) && absoluteURL[:len(baseURL)] == baseURL {
		return absoluteURL[len(baseURL):]
	}
	return absoluteURL
}
