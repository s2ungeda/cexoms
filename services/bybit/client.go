package bybit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	// API endpoints
	BaseURL         = "https://api.bybit.com"
	BaseURLTestnet  = "https://api-testnet.bybit.com"
	WSBaseURL       = "wss://stream.bybit.com/v5/public/spot"
	WSPrivateURL    = "wss://stream.bybit.com/v5/private"
	WSFuturesURL    = "wss://stream.bybit.com/v5/public/linear"

	// API version
	APIVersion = "v5"
)

// Client represents Bybit API client
type Client struct {
	apiKey     string
	apiSecret  string
	baseURL    string
	httpClient *http.Client
	testnet    bool
}

// NewClient creates a new Bybit client
func NewClient(apiKey, apiSecret string, testnet bool) *Client {
	baseURL := BaseURL
	if testnet {
		baseURL = BaseURLTestnet
	}

	return &Client{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		testnet: testnet,
	}
}

// Request makes an authenticated request to Bybit API
func (c *Client) Request(method, endpoint string, params map[string]interface{}, result interface{}) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	
	// Build query string for GET requests or body for POST
	var queryString string
	var body []byte
	var err error

	if method == http.MethodGet || method == http.MethodDelete {
		queryString = c.buildQueryString(params)
		if queryString != "" {
			endpoint = endpoint + "?" + queryString
		}
	} else {
		if params != nil {
			body, err = json.Marshal(params)
			if err != nil {
				return fmt.Errorf("failed to marshal params: %w", err)
			}
		}
	}

	// Create request
	fullURL := c.baseURL + "/" + APIVersion + endpoint
	req, err := http.NewRequest(method, fullURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BAPI-API-KEY", c.apiKey)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", "5000")

	// Generate signature
	var signPayload string
	if method == http.MethodGet || method == http.MethodDelete {
		signPayload = timestamp + c.apiKey + "5000" + queryString
	} else {
		signPayload = timestamp + c.apiKey + "5000" + string(body)
	}
	
	signature := c.sign(signPayload)
	req.Header.Set("X-BAPI-SIGN", signature)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var baseResp BaseResponse
	if err := json.Unmarshal(respBody, &baseResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API errors
	if baseResp.RetCode != 0 {
		return fmt.Errorf("API error %d: %s", baseResp.RetCode, baseResp.RetMsg)
	}

	// Unmarshal result
	if result != nil && baseResp.Result != nil {
		resultBytes, err := json.Marshal(baseResp.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// PublicRequest makes a public request (no authentication required)
func (c *Client) PublicRequest(method, endpoint string, params map[string]interface{}, result interface{}) error {
	queryString := c.buildQueryString(params)
	if queryString != "" {
		endpoint = endpoint + "?" + queryString
	}

	fullURL := c.baseURL + "/" + APIVersion + endpoint
	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var baseResp BaseResponse
	if err := json.Unmarshal(respBody, &baseResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if baseResp.RetCode != 0 {
		return fmt.Errorf("API error %d: %s", baseResp.RetCode, baseResp.RetMsg)
	}

	if result != nil && baseResp.Result != nil {
		resultBytes, err := json.Marshal(baseResp.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		if err := json.Unmarshal(resultBytes, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// sign generates HMAC SHA256 signature
func (c *Client) sign(payload string) string {
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// buildQueryString builds query string from params map
func (c *Client) buildQueryString(params map[string]interface{}) string {
	if params == nil || len(params) == 0 {
		return ""
	}

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build query string
	values := url.Values{}
	for _, k := range keys {
		v := params[k]
		switch val := v.(type) {
		case string:
			if val != "" {
				values.Add(k, val)
			}
		case int:
			values.Add(k, strconv.Itoa(val))
		case int64:
			values.Add(k, strconv.FormatInt(val, 10))
		case float64:
			values.Add(k, strconv.FormatFloat(val, 'f', -1, 64))
		case bool:
			values.Add(k, strconv.FormatBool(val))
		default:
			// Convert to JSON string for complex types
			if jsonBytes, err := json.Marshal(val); err == nil {
				values.Add(k, string(jsonBytes))
			}
		}
	}

	return values.Encode()
}

// GetServerTime gets server time
func (c *Client) GetServerTime() (int64, error) {
	var result struct {
		TimeSecond string `json:"timeSecond"`
		TimeNano   string `json:"timeNano"`
	}

	err := c.PublicRequest(http.MethodGet, "/market/time", nil, &result)
	if err != nil {
		return 0, err
	}

	timeSecond, err := strconv.ParseInt(result.TimeSecond, 10, 64)
	if err != nil {
		return 0, err
	}

	return timeSecond * 1000, nil // Convert to milliseconds
}

// GenerateSignature generates signature for WebSocket authentication
func (c *Client) GenerateSignature(expires int64) string {
	val := fmt.Sprintf("GET/realtime%d", expires)
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(val))
	return hex.EncodeToString(h.Sum(nil))
}