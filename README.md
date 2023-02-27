# Relayer service from BBC to EVM chains in order to establish a BSC light client

This is developed on [bsc-relayer](https://github.com/bnb-chain/bsc-relayer).

## Quick Start

**Note**: Requires [Go 1.19+](https://golang.org/dl/)

```go
make build

./build/bsc-relayer --bbc-network-type 0 --config-path config/config.json --log-level debug

```

## License

The library is licensed under the [GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the [LICENSE](LICENSE) file.