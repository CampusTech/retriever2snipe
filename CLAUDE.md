# CLAUDE.md

## Project Overview

retriever2snipe syncs asset records from the Retriever v2 API to Snipe-IT asset management. It's a Go CLI built with cobra.

## Build & Test

```bash
go build ./...       # Build all packages
go vet ./...         # Static analysis
go test ./...        # Run all tests
```

## Project Structure

```
main.go              # Thin entry point, calls cmd.Execute()
cmd/
  root.go            # Shared config, client constructors, Execute()
  sync.go            # sync command — main sync logic, asset mapping, CODD attachment
  setup.go           # setup command — creates Snipe-IT resources interactively
  download.go        # download command — caches Retriever API data locally
  generate_config.go # generate-config command — creates config template with model mappings
  sync_test.go       # Tests for sync logic
  root_test.go       # Tests for config loading
retriever/
  client.go          # Retriever v2 API client with pagination
  types.go           # Retriever data types (WarehouseDevice, Deployment, DeviceReturn)
  ratelimit.go       # Rate limiter for Retriever API
```

## Key Dependencies

- `github.com/michellepellon/go-snipeit` — Snipe-IT API client (CampusTech fork, `merged` branch)
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — Structured logging
- `gopkg.in/yaml.v3` — Config file parsing

## Architecture Notes

- Config precedence: CLI flags > env vars > config.yaml
- Serial number is the primary matching key between Retriever and Snipe-IT
- Sync is idempotent: compares all fields and only sends updates when data changed
- Warehouse devices are deduplicated by serial (latest initial_deployment wins)
- Notes comparison normalizes HTML entities and line endings (Snipe-IT encodes on storage)
- Snipe-IT BOOLEAN custom fields use "1"/"0", not "true"/"false"
- The go-snipeit SDK lives at /tmp/go-snipeit-upstream during development (local replace in go.mod)
- Logging uses logrus: INFO always shown, DEBUG only with --debug flag

## Conventions

- All log calls use logrus (`log "github.com/sirupsen/logrus"`), aliased as `log`
- Use `log.Infof`/`log.Info` for normal operation, `log.Debugf` for verbose/request-response, `log.Warnf` for recoverable issues, `log.Errorf` for errors that don't halt execution
- Error handling: return `fmt.Errorf("context: %w", err)` — always wrap with context
- Snipe-IT API response types: `AssetCreateResponse` for create/update/checkout/checkin, `AssetsResponse` for list/search
