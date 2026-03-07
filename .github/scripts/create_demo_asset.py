#!/usr/bin/env python3
"""
create_demo_asset.py — Create a demo fieldset, model, and asset in Snipe-IT
for README screenshot purposes.

Usage:
    SNIPE_URL=https://your-instance.snipe-it.io \\
    SNIPE_KEY=your-api-key \\
    python3 .github/scripts/create_demo_asset.py

The script is idempotent: if a fieldset/model/asset with the same name already
exists it prints the existing ID and skips creation.

To clean up afterwards, run with --delete:
    python3 .github/scripts/create_demo_asset.py --delete
"""

import json
import os
import sys
import urllib.request
import urllib.error

SNIPE_URL = os.environ.get("SNIPE_URL", "").rstrip("/")
SNIPE_KEY = os.environ.get("SNIPE_KEY", "")

if not SNIPE_URL or not SNIPE_KEY:
    print("ERROR: set SNIPE_URL and SNIPE_KEY environment variables", file=sys.stderr)
    sys.exit(1)

HEADERS = {
    "Authorization": f"Bearer {SNIPE_KEY}",
    "Accept": "application/json",
    "Content-Type": "application/json",
}

# IDs that must already exist in the Snipe-IT instance
MANUFACTURER_ID = 1   # Apple
STATUS_ID       = 2   # Ready to Deploy
CATEGORY_ID     = 2   # Computers

# Retriever custom field IDs created by `retriever2snipe setup`
RETRIEVER_FIELD_IDS = [33, 34, 35, 36, 37]
# Built-in field IDs: MAC Address, RAM, Storage
BUILTIN_FIELD_IDS = [1, 2, 3]

FIELDSET_NAME = "retriever2snipe Demo"
MODEL_NAME    = "MacBook Air (13-inch) [M4] (Demo)"
ASSET_TAG     = "DEMO-MBA-001"
SERIAL        = "DEMO12345XYZ"


def api(method, path, body=None, fatal=True):
    url = f"{SNIPE_URL}/api/v1{path}"
    data = json.dumps(body).encode() if body else None
    req = urllib.request.Request(url, data=data, headers=HEADERS, method=method)
    try:
        with urllib.request.urlopen(req) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        msg = f"HTTP {e.code} {method} {path}: {e.read().decode()}"
        if fatal:
            print(msg, file=sys.stderr)
            sys.exit(1)
        print(f"  WARNING: {msg}", file=sys.stderr)
        return {}


def find_by_name(rows, name):
    for r in rows:
        if r.get("name") == name:
            return r
    return None


def create():
    # ── 1. Fieldset ───────────────────────────────────────────────────────────
    print(f"\n[1/3] Fieldset: {FIELDSET_NAME!r}")
    fs = find_by_name(api("GET", "/fieldsets")["rows"], FIELDSET_NAME)
    if fs:
        fieldset_id = fs["id"]
        print(f"  → already exists (id={fieldset_id}), skipping")
    else:
        result = api("POST", "/fieldsets", {"name": FIELDSET_NAME})
        if result.get("status") != "success":
            print(f"  ERROR: {result}", file=sys.stderr); sys.exit(1)
        fieldset_id = result["payload"]["id"]
        print(f"  → created (id={fieldset_id})")
        for fid in BUILTIN_FIELD_IDS + RETRIEVER_FIELD_IDS:
            r = api("POST", f"/fields/{fid}/associate", {"fieldset_id": fieldset_id})
            print(f"     associate field {fid}: {r.get('status', '?')}")

    # ── 2. Model ──────────────────────────────────────────────────────────────
    print(f"\n[2/3] Model: {MODEL_NAME!r}")
    mdl = find_by_name(api("GET", "/models?limit=500")["rows"], MODEL_NAME)
    if mdl:
        model_id = mdl["id"]
        print(f"  → already exists (id={model_id}), skipping")
    else:
        result = api("POST", "/models", {
            "name":            MODEL_NAME,
            "manufacturer_id": MANUFACTURER_ID,
            "category_id":     CATEGORY_ID,
            "fieldset_id":     fieldset_id,
        })
        if result.get("status") != "success":
            print(f"  ERROR: {result}", file=sys.stderr); sys.exit(1)
        model_id = result["payload"]["id"]
        print(f"  → created (id={model_id})")

    # ── 3. Asset ──────────────────────────────────────────────────────────────
    print(f"\n[3/3] Asset: {ASSET_TAG!r} (serial {SERIAL})")
    existing = api("GET", f"/hardware/byserial/{SERIAL}")
    if existing.get("total", 0) > 0:
        asset_id = existing["rows"][0]["id"]
        print(f"  → already exists (id={asset_id}), skipping")
    else:
        notes = (
            "=== retriever2snipe:notes-start ===\n"
            "<strong>Device Info</strong>\n"
            "Location: Retriever Warehouse\n"
            "Source: IT Department\n"
            "\n"
            "<strong>Latest Deployment</strong>\n"
            "Deployed to: Jane Smith (jane.smith@example.com)\n"
            "Shipped: 2025-11-15 via FedEx (tracking: 7948271635)\n"
            "Status: delivered\n"
            "\n"
            "<strong>Latest Return</strong>\n"
            "Returned by: Jane Smith (jane.smith@example.com)\n"
            "Return initiated: 2026-01-20\n"
            "Status: device_received\n"
            "=== retriever2snipe:notes-end ==="
        )
        result = api("POST", "/hardware", {
            "asset_tag":      ASSET_TAG,
            "serial":         SERIAL,
            "name":           "Apple MacBook Air (13-inch) [M4]",
            "model_id":       model_id,
            "status_id":      STATUS_ID,
            "notes":          notes,
            "_snipeit_ram_2":                                       "8192",
            "_snipeit_storage_3":                                   "256",
            "_snipeit_retriever_id_33":                             "ABC12DEF",
            "_snipeit_retriever_has_charger_34":                    "1",
            "_snipeit_retriever_certificate_of_data_destruction_35": "https://drive.google.com/file/d/1abc123/view",
            "_snipeit_retriever_legal_hold_36":                     "0",
            "_snipeit_retriever_rating_37":                         "Good",
        })
        if result.get("status") != "success":
            print(f"  ERROR: {result}", file=sys.stderr); sys.exit(1)
        asset_id = result["payload"]["id"]
        print(f"  → created (id={asset_id})")

    print(f"\nDone.")
    print(f"  Asset:    {SNIPE_URL}/hardware/{asset_id}")
    print(f"  Fieldset: {SNIPE_URL}/fields/fieldsets/{fieldset_id}/edit")


def delete():
    print("Deleting demo data...\n")

    # Delete asset by serial
    existing = api("GET", f"/hardware/byserial/{SERIAL}")
    if existing.get("total", 0) > 0:
        asset_id = existing["rows"][0]["id"]
        api("DELETE", f"/hardware/{asset_id}", fatal=False)
        print(f"  Deleted asset id={asset_id}")
    else:
        print("  Asset not found, skipping")

    # Delete model by name
    mdl = find_by_name(api("GET", "/models?limit=500")["rows"], MODEL_NAME)
    if mdl:
        api("DELETE", f"/models/{mdl['id']}", fatal=False)
        print(f"  Deleted model id={mdl['id']}")
    else:
        print("  Model not found, skipping")

    # Delete fieldset by name (must disassociate all fields first)
    fs = find_by_name(api("GET", "/fieldsets")["rows"], FIELDSET_NAME)
    if fs:
        fid = fs["id"]
        for field in api("GET", f"/fieldsets/{fid}").get("fields", {}).get("rows", []):
            api("POST", f"/fields/{field['id']}/disassociate", {"fieldset_id": fid}, fatal=False)
        api("DELETE", f"/fieldsets/{fid}", fatal=False)
        print(f"  Deleted fieldset id={fid}")
    else:
        print("  Fieldset not found, skipping")

    print("\nDone.")


if "--delete" in sys.argv:
    delete()
else:
    create()
