#!/usr/bin/env python3

import json
import os
import requests
from datetime import datetime

VAULT_ADDR = "http://localhost:8200"
VAULT_KEYS_FILE = os.path.expanduser("~/.mExOms/vault-keys.json")
BACKUP_FILE = os.path.expanduser("~/.mExOms/api-keys-backup.json")

# Read Vault keys
if not os.path.exists(VAULT_KEYS_FILE):
    print("No Vault keys found")
    exit(1)

with open(VAULT_KEYS_FILE) as f:
    vault_keys = json.load(f)
    
root_token = vault_keys.get('root_token')

print("Backing up API keys...")

# Get Spot keys
headers = {"X-Vault-Token": root_token}
spot_resp = requests.get(f"{VAULT_ADDR}/v1/secret/data/exchanges/binance_spot", headers=headers)
spot_data = {}
if spot_resp.status_code == 200:
    spot_data = spot_resp.json().get('data', {}).get('data', {})

# Get Futures keys  
futures_resp = requests.get(f"{VAULT_ADDR}/v1/secret/data/exchanges/binance_futures", headers=headers)
futures_data = {}
if futures_resp.status_code == 200:
    futures_data = futures_resp.json().get('data', {}).get('data', {})

# Create backup
backup = {
    "spot": spot_data,
    "futures": futures_data,
    "backup_date": datetime.utcnow().isoformat() + "Z"
}

# Save backup
os.makedirs(os.path.dirname(BACKUP_FILE), exist_ok=True)
with open(BACKUP_FILE, 'w') as f:
    json.dump(backup, f, indent=2)
    
os.chmod(BACKUP_FILE, 0o600)
print(f"API keys backed up to {BACKUP_FILE}")