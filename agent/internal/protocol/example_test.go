package protocol

import (
	"context"
	"fmt"

	"github.com/nodexeus/agent/internal/config"
)

// Example demonstrates how to use the protocol module system
func Example() {
	// Create a new protocol registry
	registry := NewRegistry()

	// Register protocol modules
	registry.Register(NewEthereumModule())
	registry.Register(NewArbitrumModule())

	// Set up the config validator
	config.SetProtocolValidator(registry)

	// Check if a protocol is registered
	if registry.IsRegistered("ethereum") {
		fmt.Println("Ethereum protocol is registered")
	}

	// Get a protocol module
	ethModule, err := registry.Get("ethereum")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Retrieved module: %s\n", ethModule.Name())

	// Example of collecting metrics (would need a real RPC endpoint)
	ctx := context.Background()
	nodeConfig := config.NodeConfig{
		Protocol:  "ethereum",
		Type:      "archive",
		RPCUrl:    "http://localhost:8545",
		BeaconUrl: "http://localhost:5052",
	}

	// Note: This would return metrics with nil values without a real RPC endpoint
	metrics, err := ethModule.CollectMetrics(ctx, nodeConfig)
	if err == nil && metrics != nil {
		fmt.Println("Metrics collection would require a real RPC endpoint")
	}

	// Output:
	// Ethereum protocol is registered
	// Retrieved module: ethereum
	// Metrics collection would require a real RPC endpoint
}
