# retriever2snipe

Sync asset records from the [Retriever](https://helloretriever.com/) v2 API to [Snipe-IT](https://snipeitapp.com/) asset management. Inspired by [jamf2snipe](https://github.com/grokability/jamf2snipe).

## Features

- Syncs warehouse device inventory from Retriever to Snipe-IT assets
- Enriches assets with deployment and return tracking data
- Matches assets by serial number (creates new or updates existing)
- Maps all Retriever device statuses to Snipe-IT status labels (one-to-one)
- Sets Snipe-IT location for devices physically at the Retriever warehouse
- Custom field support: Retriever ID, Has Charger, Certificate of Data Destruction (CODD), Legal Hold, Rating
- Maps Retriever hardware specs (RAM, storage, processor, screen size, OS) to existing Snipe-IT custom fields
- Automatically downloads and attaches CODD PDFs from Google Drive to Snipe-IT assets
- Interactive `setup` command to auto-create Snipe-IT resources (locations, status labels, custom fields, fieldsets)
- Config file generation from cached Retriever data with model mapping
- Local data caching to avoid hitting Retriever's strict rate limits during development
- Dry-run mode with optional debug payload output
- Single-device sync by serial number or Retriever device ID
- Optional Slack webhook notifications when new assets are created
- Idempotent sync: only updates assets when data has actually changed
- Deduplicates warehouse records by serial number (keeps latest deployment)

## Requirements

- Go 1.25 or later
- A [Retriever](https://helloretriever.com/) account with an API key
- A [Snipe-IT](https://snipeitapp.com/) instance with an API key

## Installation

```bash
go install github.com/CampusTech/retriever2snipe@latest
```

Or build from source:

```bash
git clone https://github.com/CampusTech/retriever2snipe.git
cd retriever2snipe
go build -o retriever2snipe .
```

## Quick Start

### 1. Set up Snipe-IT resources

The `setup` command interactively creates the warehouse location, status labels, custom fields, and fieldset in Snipe-IT, then writes a config file:

```bash
retriever2snipe setup \
  --snipeit-url="https://your-instance.snipeitapp.com" \
  --snipeit-api-key="$SNIPEIT_API_KEY"
```

This creates:

- A "Retriever Warehouse" location
- Status labels for all 21 Retriever device statuses
- Custom fields:
  - Retriever: ID,
  - Retriever: Has Charger?,
  - Retriever: Certificate of Data Destruction
  - Retriever: Legal Hold
  - Retriever: Rating
- A fieldset grouping those custom fields (you choose an existing one or create new)

The resulting IDs are saved to `config.yaml`.

### 2. Download data for local development

Retriever enforces strict rate limits (60 requests/minute, 500 requests/day). Download all data once and work from the cache:

```bash
retriever2snipe download --retriever-api-key="$RETRIEVER_API_KEY"
```

This saves `warehouse.json`, `deployments.json`, and `device_returns.json` to the cache directory.

### 3. Generate model mappings

After downloading, generate a config template that pre-fills model mappings from your Retriever inventory:

```bash
retriever2snipe generate-config --use-cache
```

Edit the generated file to map each Retriever "Manufacturer Model" to a Snipe-IT model ID.

### 4. Preview the sync (dry run)

```bash
retriever2snipe sync --use-cache --dry-run --debug
```

### 5. Run the sync

```bash
retriever2snipe sync --use-cache
```

Or sync directly from the Retriever API (without cache):

```bash
retriever2snipe sync --retriever-api-key="$RETRIEVER_API_KEY"
```

## Configuration

Configuration is loaded in order of precedence: CLI flags > environment variables > config file (`config.yaml`).

See [`config.yaml.example`](config.yaml.example) for all available options.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `RETRIEVER_API_KEY` | Retriever API key |
| `SNIPEIT_API_KEY` | Snipe-IT API key |
| `SNIPEIT_URL` | Snipe-IT base URL |
| `RETRIEVER_CACHE_DIR` | Cache directory (default: `.cache`) |
| `SLACK_WEBHOOK_URL` | Slack webhook URL for new asset notifications |

### Commands and Flags

#### Global

| Flag | Description |
|------|-------------|
| `--config` | Path to YAML config file (default: `config.yaml`) |

#### `setup`

| Flag | Description |
|------|-------------|
| `--snipeit-api-key` | Snipe-IT API key |
| `--snipeit-url` | Snipe-IT base URL |
| `--output`, `-o` | Output config file path (default: same as `--config`) |
| `--dry-run` | Preview what would be created without making changes |

#### `download`

| Flag | Description |
|------|-------------|
| `--retriever-api-key` | Retriever API key |
| `--cache-dir` | Cache directory |

#### `generate-config`

| Flag | Description |
|------|-------------|
| `--retriever-api-key` | Retriever API key |
| `--cache-dir` | Cache directory |
| `--use-cache` | Use cached data instead of fetching from API |

#### `sync`

| Flag | Description |
|------|-------------|
| `--retriever-api-key` | Retriever API key |
| `--snipeit-api-key` | Snipe-IT API key |
| `--snipeit-url` | Snipe-IT base URL |
| `--cache-dir` | Cache directory |
| `--use-cache` | Use cached data instead of fetching from API |
| `--dry-run` | Preview changes without writing to Snipe-IT |
| `--debug` | Enable debug logging (API request/response bodies, update reasons) |
| `--default-status-id` | Snipe-IT status label ID for new assets (required) |
| `--default-model-id` | Snipe-IT model ID when no model match is found |
| `--warehouse-location-id` | Snipe-IT location ID for Retriever Warehouse |
| `--serial` | Sync only the device with this serial number |
| `--retriever-device-id` | Sync only the device with this Retriever device ID |
| `--slack-webhook-url` | Slack webhook URL for new asset notifications |

## Status Mapping

The `setup` command creates a Snipe-IT status label for each Retriever status. Each status is assigned a Snipe-IT type:

| Snipe-IT Type | Retriever Statuses |
|---|---|
| Deployable | `deployed`, `ready_for_deployment` |
| Pending | `deployment_initiated`, `deployment_requested`, `device_received`, `input_required`, `label_generated`, `provisioning`, `returned`, `return_initiated`, `retrieval_initiated` |
| Archived | `delivered_address`, `disposal_initiated`, `disposed`, `returned_to_vendor`, `to_be_retired` |
| Undeployable | `administrative_hold`, `in_repair`, `legal_hold`, `lost_in_transit`, `requires_service` |

### Warehouse Location

Devices with the following statuses are assigned to the Retriever Warehouse location:

`device_received`, `ready_for_deployment`, `provisioning`, `in_repair`, `requires_service`, `administrative_hold`, `legal_hold`, `input_required`

## Asset Field Mapping

| Snipe-IT Field | Source |
|---|---|
| Serial | `serial_number` |
| Asset Tag | `asset_tag` (falls back to serial number) |
| Name | `{manufacturer} {model}` (preserved if already set in Snipe-IT) |
| Status Label | Mapped from Retriever status via `status_mappings` config |
| Location | Retriever Warehouse if device is in warehouse |
| Model | Matched via `model_mappings` config |
| Notes | Device notes, location, source, deployment/return tracking info |

### Custom Fields

The `setup` command creates these Retriever-specific fields:

| Field | Source | Format |
|---|---|---|
| Retriever: ID | `id` | Text |
| Retriever: Has Charger? | `has_charger` | Boolean |
| Retriever: Certificate of Data Destruction | `codd` | URL |
| Retriever: Legal Hold | `legal_hold` | Numeric (days) |
| Retriever: Rating | `rating` | Text |

CODD values are sourced from both warehouse device data and device returns. When a CODD URL is a Google Drive link, the PDF is downloaded and attached to the asset in Snipe-IT.

You can also map Retriever hardware specs to your existing Snipe-IT custom fields via the `custom_fields` config. For example, to populate your existing RAM and Storage fields:

```yaml
custom_fields:
  _snipeit_ram_2: ram          # converted to MB automatically
  _snipeit_storage_3: disk     # kept in GB
```

All supported Retriever fields: `id`, `has_charger`, `codd`, `legal_hold`, `rating`, `ram`, `disk`, `os`, `os_version`, `screen_size`, `processor`, `serial_number`, `manufacturer`, `model`, `device_series`, `release_year`, `status`, `current_location`, `asset_tag`, `recipient_source`, `notes`.

## Rate Limits

- **Retriever API**: 60 read requests/minute, 500 read requests/day
- The built-in client limits itself to 50 requests/minute to stay safely under the limit
- **Snipe-IT API**: The client uses a token bucket rate limiter (2 requests/second, burst of 5)
- Use `download` + `--use-cache` during development to avoid burning your daily Retriever quota

## License

[MIT](LICENSE.md)
