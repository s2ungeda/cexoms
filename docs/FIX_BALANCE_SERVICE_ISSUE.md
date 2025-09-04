# Balance Service API Key Issue - Root Cause and Solution

## Problem Summary
The `make start-all` command was failing to retrieve API keys from Vault, causing the balance service to repeatedly log "API-key format invalid" errors.

## Root Cause Analysis

### 1. Compilation Error in Security Package
The original balance service (`cmd/binance-spot-balance/main.go`) depends on the `pkg/security` package, which has multiple compilation errors:

```
pkg/security/threat_intelligence.go:83:2: FormatJSON redeclared in this block
pkg/security/anomaly_detector.go:620:5: undefined: strings
pkg/security/compliance_manager.go:451:2: declared and not used: exportData
... (and more)
```

### 2. Service Cannot Build
Because of these compilation errors, the original balance service cannot be built. When `start-oms.sh` attempts to run it, it either:
- Falls back to an older binary that uses environment variables
- Uses placeholder values from `.env` file ("your_api_key_here")

### 3. Placeholder API Keys
The `.env` file contains placeholder values that are obviously invalid for Binance API authentication.

## Solution Implemented

### 1. Created Simple Balance Service
A new simplified balance service was created at `cmd/binance-spot-balance-simple/main.go` that:
- Directly retrieves API keys from Vault using HTTP API
- Does not depend on the broken security package
- Successfully authenticates with Binance and retrieves balance data

### 2. Updated Start Script
The `start-oms.sh` script has been updated (line 89) to use the working balance service:
```bash
go run cmd/binance-spot-balance-simple/main.go > logs/balance.log 2>&1 &
```

### 3. Verification
The simple balance service is now successfully:
- Retrieving API keys from Vault
- Fetching balance data from Binance (showing ~$3415 in total assets)
- Publishing balance updates to NATS every 10 seconds
- Dashboard receiving and displaying the balance data

## Permanent Fix Options

### Option 1: Fix Security Package (Recommended)
Fix the compilation errors in the security package:
- Add missing imports (`strings` package)
- Remove duplicate function declarations
- Fix unused variables
- Fix undefined methods

### Option 2: Keep Simple Service
Continue using the simplified balance service which:
- Works reliably
- Has fewer dependencies
- Is easier to maintain

### Option 3: Create Dedicated Vault Package
Extract just the Vault client functionality into a separate, minimal package without the other security features that are causing compilation issues.

## Lessons Learned

1. **Dependency Management**: Complex packages with many features can become maintenance burdens
2. **Fallback Behavior**: Services should fail clearly rather than falling back to invalid placeholder values
3. **Testing**: Build tests should be run before deployment to catch compilation errors
4. **Simplicity**: Sometimes a simple solution is better than a complex one

## Commands for Testing

To verify the fix is working:
```bash
# Check if balance service is retrieving keys successfully
tail -f logs/balance-simple.log | grep "Successfully retrieved API keys"

# Check if balances are being published
tail -f logs/balance-simple.log | grep "Published spot balances"

# Verify dashboard is receiving balance data
curl -s http://localhost:8080/ws
```