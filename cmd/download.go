package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// NewDownloadCmd creates the download command.
func NewDownloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download",
		Short: "Download Retriever data to local cache for development",
		Long:  "Fetches all warehouse devices and deployments from the Retriever API and saves them as JSON files. Use this to avoid hitting rate limits during development.",
		RunE:  runDownload,
	}
}

func runDownload(cmd *cobra.Command, args []string) error {
	if Cfg.RetrieverAPIKey == "" {
		return fmt.Errorf("retriever API key is required (--retriever-api-key or RETRIEVER_API_KEY)")
	}

	client := NewRetrieverClient()

	if err := os.MkdirAll(Cfg.CacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	ctx := context.Background()

	// Download warehouse devices
	log.Info("Downloading warehouse devices...")
	devices, err := client.ListAllWarehouseDevices(ctx)
	if err != nil {
		return fmt.Errorf("fetching warehouse devices: %w", err)
	}
	log.Infof("Downloaded %d warehouse devices", len(devices))

	if err := WriteJSON(filepath.Join(Cfg.CacheDir, "warehouse.json"), devices); err != nil {
		return fmt.Errorf("writing warehouse cache: %w", err)
	}

	// Download deployments
	log.Info("Downloading deployments...")
	deployments, err := client.ListAllDeployments(ctx)
	if err != nil {
		return fmt.Errorf("fetching deployments: %w", err)
	}
	log.Infof("Downloaded %d deployments", len(deployments))

	if err := WriteJSON(filepath.Join(Cfg.CacheDir, "deployments.json"), deployments); err != nil {
		return fmt.Errorf("writing deployments cache: %w", err)
	}

	// Download device returns
	log.Info("Downloading device returns...")
	returns, err := client.ListAllDeviceReturns(ctx)
	if err != nil {
		return fmt.Errorf("fetching device returns: %w", err)
	}
	log.Infof("Downloaded %d device returns", len(returns))

	if err := WriteJSON(filepath.Join(Cfg.CacheDir, "device_returns.json"), returns); err != nil {
		return fmt.Errorf("writing device returns cache: %w", err)
	}

	log.Info("Download complete. Use --use-cache with sync to use cached data.")
	return nil
}
