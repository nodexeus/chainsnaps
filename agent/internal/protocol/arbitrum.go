package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/yourusername/snapd/internal/config"
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

// CollectMetrics executes Arbitrum-specific RPC queries
func (a *ArbitrumModule) CollectMetrics(ctx context.Context, cfg config.NodeConfig) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// Query eth_blockNumber from Arbitrum node
	blockNumber, err := a.queryBlockNumber(ctx, cfg.RPCUrl)
	if err != nil {
		metrics["latest_block"] = nil
	} else {
		metrics["latest_block"] = blockNumber
	}

	return metrics, nil
}

// queryBlockNumber queries the latest block number via JSON-RPC
func (a *ArbitrumModule) queryBlockNumber(ctx context.Context, rpcURL string) (string, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var response struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("RPC error: %s", response.Error.Message)
	}

	return response.Result, nil
}
