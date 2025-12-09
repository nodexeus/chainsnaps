# Protocol Module System

This package implements the protocol module system for the Snapshot Daemon, providing a pluggable architecture for blockchain-specific metric collection.

## Components

### ProtocolModule Interface

The core interface that all protocol modules must implement:

```go
type ProtocolModule interface {
    Name() string
    CollectMetrics(ctx context.Context, config config.NodeConfig) (map[string]interface{}, error)
}
```

### Registry

The `Registry` provides thread-safe registration and retrieval of protocol modules:

- `Register(module ProtocolModule)` - Register a new protocol module
- `Get(name string)` - Retrieve a protocol module by name
- `IsRegistered(name string)` - Check if a protocol is registered
- `List()` - Get all registered protocol names

### Implemented Protocol Modules

#### Ethereum Module

Collects metrics from Ethereum nodes:
- `latest_block` - Latest block number from execution client (eth_blockNumber)
- `latest_slot` - Latest beacon chain slot (if beacon URL provided)
- `earliest_blob` - Earliest blob index (if beacon URL provided)

#### Arbitrum Module

Collects metrics from Arbitrum nodes:
- `latest_block` - Latest block number (eth_blockNumber)

## Usage

```go
// Create registry
registry := protocol.NewRegistry()

// Register modules
registry.Register(protocol.NewEthereumModule())
registry.Register(protocol.NewArbitrumModule())

// Set up config validation
config.SetProtocolValidator(registry)

// Get a module
module, err := registry.Get("ethereum")
if err != nil {
    log.Fatal(err)
}

// Collect metrics
ctx := context.Background()
metrics, err := module.CollectMetrics(ctx, nodeConfig)
if err != nil {
    log.Printf("Failed to collect metrics: %v", err)
}
```

## Configuration Integration

The protocol system integrates with the configuration package to validate that all configured node protocols have registered modules. This validation happens during configuration loading:

```yaml
nodes:
  ethereum-mainnet:
    protocol: ethereum  # Must be registered
    rpc_url: http://localhost:8545
    beacon_url: http://localhost:5052
```

## Error Handling

- Failed RPC queries result in `nil` values for the affected metrics
- The system continues collecting other metrics even if some fail
- All errors are returned with context for debugging

## Testing

The package includes comprehensive tests:
- Unit tests for registry operations
- Tests for protocol module interface compliance
- Integration tests with config validation
- Mock implementations for testing

Run tests with:
```bash
go test ./internal/protocol/...
```

## Requirements Satisfied

This implementation satisfies the following requirements:

- **3.1**: Protocol modules are invoked based on node's protocol field
- **3.2**: Protocol modules return map of metric names to values
- **4.1**: Common ProtocolModule interface defined
- **4.2**: Protocol modules receive node configuration as input
- **4.3**: Protocol modules return structured metric data
- **4.4**: Protocol modules are registered at daemon startup
- **4.5**: Unregistered protocols are validated during config loading
