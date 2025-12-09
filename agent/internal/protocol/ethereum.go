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

// EthereumModule implements the ProtocolModule interface for Ethereum nodes
type EthereumModule struct {
	httpClient *http.Client
}

// NewEthereumModule creates a new Ethereum protocol module
func NewEthereumModule() *EthereumModule {
	return &EthereumModule{
		httpClient: &http.Client{},
	}
}

// Name returns the protocol identifier
func (e *EthereumModule) Name() string {
	return "ethereum"
}

// CollectMetrics executes Ethereum-specific RPC queries
func (e *EthereumModule) CollectMetrics(ctx context.Context, cfg config.NodeConfig) (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// Query eth_blockNumber from execution client
	blockNumber, err := e.queryBlockNumber(ctx, cfg.RPCUrl)
	if err != nil {
		metrics["latest_block"] = nil
	} else {
		metrics["latest_block"] = blockNumber
	}

	// Query beacon chain slot if beacon URL is provided
	if cfg.BeaconUrl != "" {
		slot, err := e.queryBeaconSlot(ctx, cfg.BeaconUrl)
		if err != nil {
			metrics["latest_slot"] = nil
		} else {
			metrics["latest_slot"] = slot
		}

		// Query earliest blob
		earliestBlob, err := e.queryEarliestBlob(ctx, cfg.BeaconUrl)
		if err != nil {
			metrics["earliest_blob"] = nil
		} else {
			metrics["earliest_blob"] = earliestBlob
		}
	}

	return metrics, nil
}

// queryBlockNumber queries the latest block number via JSON-RPC
func (e *EthereumModule) queryBlockNumber(ctx context.Context, rpcURL string) (string, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}

	respData, err := e.doJSONRPCRequest(ctx, rpcURL, reqBody)
	if err != nil {
		return "", err
	}

	var response struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respData, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("RPC error: %s", response.Error.Message)
	}

	return response.Result, nil
}

// queryBeaconSlot queries the latest beacon chain slot
func (e *EthereumModule) queryBeaconSlot(ctx context.Context, beaconURL string) (int64, error) {
	url := fmt.Sprintf("%s/eth/v1/beacon/headers/head", beaconURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var response struct {
		Data struct {
			Header struct {
				Message struct {
					Slot string `json:"slot"`
				} `json:"message"`
			} `json:"header"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	var slot int64
	if _, err := fmt.Sscanf(response.Data.Header.Message.Slot, "%d", &slot); err != nil {
		return 0, fmt.Errorf("failed to parse slot: %w", err)
	}

	return slot, nil
}

// queryEarliestBlob queries the earliest blob slot
func (e *EthereumModule) queryEarliestBlob(ctx context.Context, beaconURL string) (int64, error) {
	url := fmt.Sprintf("%s/eth/v1/beacon/blob_sidecars/finalized", beaconURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var response struct {
		Data []struct {
			Index string `json:"index"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Data) == 0 {
		return 0, fmt.Errorf("no blob data available")
	}

	// Find the earliest (minimum) index
	var earliestIndex int64 = -1
	for _, blob := range response.Data {
		var index int64
		if _, err := fmt.Sscanf(blob.Index, "%d", &index); err != nil {
			continue
		}
		if earliestIndex == -1 || index < earliestIndex {
			earliestIndex = index
		}
	}

	if earliestIndex == -1 {
		return 0, fmt.Errorf("failed to parse any blob indices")
	}

	return earliestIndex, nil
}

// doJSONRPCRequest performs a JSON-RPC request
func (e *EthereumModule) doJSONRPCRequest(ctx context.Context, url string, reqBody map[string]interface{}) ([]byte, error) {
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
