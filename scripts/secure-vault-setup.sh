#!/bin/bash

# Secure Vault Setup with Encryption
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== Secure Vault Setup ===${NC}"

# 1. Setup encrypted storage directory
VAULT_BASE="$HOME/.mExOms/secure-vault"
mkdir -p $VAULT_BASE/{data,config,keys}
chmod 700 $VAULT_BASE

# 2. Generate strong encryption key for Vault
if [ ! -f "$VAULT_BASE/keys/vault-seal.key" ]; then
    echo -e "\n${GREEN}Generating encryption keys...${NC}"
    openssl rand -base64 32 > $VAULT_BASE/keys/vault-seal.key
    chmod 600 $VAULT_BASE/keys/vault-seal.key
fi

# 3. Create secure Vault configuration
cat > $VAULT_BASE/config/vault.hcl << 'EOF'
ui = true

# Encrypted file storage
storage "file" {
  path = "/vault/data"
}

# Auto-unseal using transit secrets engine
seal "transit" {
  disable_renewal = "false"
  key_name        = "autounseal"
  mount_path      = "transit/"
}

listener "tcp" {
  address = "0.0.0.0:8200"
  tls_cert_file = "/vault/config/vault.crt"
  tls_key_file = "/vault/config/vault.key"
}

telemetry {
  prometheus_retention_time = "30s"
  disable_hostname = true
}

log_level = "info"
api_addr = "https://0.0.0.0:8200"
cluster_addr = "https://0.0.0.0:8201"
EOF

# 4. Generate self-signed TLS certificates
if [ ! -f "$VAULT_BASE/config/vault.crt" ]; then
    echo -e "\n${GREEN}Generating TLS certificates...${NC}"
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
        -keyout $VAULT_BASE/config/vault.key \
        -out $VAULT_BASE/config/vault.crt \
        -subj "/C=US/ST=State/L=City/O=OMS/CN=vault.local"
    chmod 600 $VAULT_BASE/config/vault.key
fi

# 5. Create systemd service for auto-start
sudo tee /etc/systemd/system/vault-oms.service > /dev/null << EOF
[Unit]
Description=HashiCorp Vault for OMS
After=docker.service
Requires=docker.service

[Service]
Type=simple
User=$USER
ExecStartPre=/usr/bin/docker pull hashicorp/vault:latest
ExecStart=/usr/bin/docker run --rm --name vault-oms \
    -p 8200:8200 \
    -v $VAULT_BASE/data:/vault/data \
    -v $VAULT_BASE/config:/vault/config \
    --cap-add IPC_LOCK \
    hashicorp/vault:latest server -config=/vault/config/vault.hcl
ExecStop=/usr/bin/docker stop vault-oms
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 6. Create secure key storage script
cat > scripts/secure-store-keys.sh << 'EOF'
#!/bin/bash

# Secure key storage with encryption
set -e

echo "=== Secure API Key Storage ==="

# Get master password
echo -n "Enter master password for key encryption: "
read -s MASTER_PASS
echo

# Get API keys
echo -e "\nEnter Binance API credentials:"
read -p "API Key: " API_KEY
echo -n "Secret Key: "
read -s SECRET_KEY
echo

# Encrypt keys before storing
ENCRYPTED_API=$(echo -n "$API_KEY" | openssl enc -aes-256-cbc -a -salt -pass pass:"$MASTER_PASS" -pbkdf2)
ENCRYPTED_SECRET=$(echo -n "$SECRET_KEY" | openssl enc -aes-256-cbc -a -salt -pass pass:"$MASTER_PASS" -pbkdf2)

# Store in Vault
export VAULT_ADDR='https://localhost:8200'
export VAULT_SKIP_VERIFY=1
VAULT_TOKEN=$(cat $HOME/.mExOms/secure-vault/keys/vault-root-token 2>/dev/null)

if [ -z "$VAULT_TOKEN" ]; then
    echo "Error: Vault not initialized. Run secure-vault-setup.sh first"
    exit 1
fi

vault kv put secret/exchanges/binance_spot \
    api_key="$ENCRYPTED_API" \
    secret_key="$ENCRYPTED_SECRET" \
    encrypted="true" \
    algorithm="aes-256-cbc"

echo "✅ Keys stored securely in Vault (encrypted)"
EOF
chmod +x scripts/secure-store-keys.sh

echo -e "\n${GREEN}✅ Secure Vault setup complete!${NC}"
echo -e "\nNext steps:"
echo -e "1. Start Vault: ${YELLOW}sudo systemctl start vault-oms${NC}"
echo -e "2. Initialize Vault: ${YELLOW}./scripts/init-secure-vault.sh${NC}"
echo -e "3. Store API keys: ${YELLOW}./scripts/secure-store-keys.sh${NC}"