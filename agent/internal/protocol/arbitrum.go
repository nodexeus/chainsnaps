package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nodexeus/agent/internal/config"
)

// ArbitrumModule implements the ProtocolModule interface for Arbitrum nodes
type ArbitrumModule struct {
	httpClient *http.Client
}

// NewArbitrumModule creates a new Arbitrum protocol module
func NewArbitrumModule() *ArbitrumModule {
	return &ArbitrumModule{
		httpClient: &http.Client{},
	}
}

// Name returns the protocol identifier
func (a *ArbitrumModule) Name() string {
	return "arbitrum"
}

// Aliases returns alternative protocol identifiers that map to this module.
func (a *ArbitrumModule) Aliases() []string {
	return []string{"arbitrum-one"}
}

// CollectMetrics executes Arbitrum-specific RPC queries
func (a *ArbitrumModule) CollectMetrics(ctx context.Context, cfg config.NodeConfig) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// Query eth_blockNumber from Arbitrum node
	blockNumber, err := a.queryBlockNumber(ctx, cfg.URL)
	if err != nil {
		metrics["latest_block"] = nil
	} else {
		metrics["latest_block"] = blockNumber
	}

	return metrics, nil
}

// queryBlockNumber queries the latest block number via JSON-RPC
func (e *ArbitrumModule) queryBlockNumber(ctx context.Context, rpcURL string) (int64, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	respData, err := e.doJSONRPCRequest(ctx, rpcURL, reqBody)
	if err != nil {
		return 0, err
	}

	var response struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return 0, fmt.Errorf("RPC error: %s", response.Error.Message)
	}

	// Convert hexadecimal string to decimal
	blockNumber, err := e.hexToInt64(response.Result)
	if err != nil {
		return 0, fmt.Errorf("failed to convert hex block number to decimal: %w", err)
	}

	return blockNumber, nil
}

// doJSONRPCRequest performs a JSON-RPC request
func (e *ArbitrumModule) doJSONRPCRequest(ctx context.Context, url string, reqBody map[string]interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// hexToInt64 converts a hexadecimal string (with or without 0x prefix) to int64
func (e *ArbitrumModule) hexToInt64(hexStr string) (int64, error) {
	// Remove 0x prefix if present
	hexStr = strings.TrimPrefix(hexStr, "0x")

	// Parse as hexadecimal
	value, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex string '%s': %w", hexStr, err)
	}

	return value, nil
}
