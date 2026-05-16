## Foundry

**Foundry is a blazing fast, portable and modular toolkit for Ethereum application development written in Rust.**

Foundry consists of:

- **Forge**: Ethereum testing framework (like Truffle, Hardhat and DappTools).
- **Cast**: Swiss army knife for interacting with EVM smart contracts, sending transactions and getting chain data.
- **Anvil**: Local Ethereum node, akin to Ganache, Hardhat Network.
- **Chisel**: Fast, utilitarian, and verbose solidity REPL.

## Documentation

https://book.getfoundry.sh/

## Usage

### Build

```shell
$ forge build
```

### Test

```shell
$ forge test
```

### Format

```shell
$ forge fmt
```

### Gas Snapshots

```shell
$ forge snapshot
```

### Anvil

```shell
$ anvil
```

### Deploy

```shell
$ FOUNDRY_PROFILE=0g_mainnet forge script script/Deploy.s.sol:DeployScript --broadcast
```

Deploy only `Escrow` with an existing ERC20 token:

```shell
$ FOUNDRY_PROFILE=0g_mainnet forge script script/DeployEscrow.s.sol:DeployEscrowScript --broadcast
```

Deploy only `NettingSettlement` with an existing ERC20 token:

```shell
$ FOUNDRY_PROFILE=0g_mainnet forge script script/DeployNettingSettlement.s.sol:DeployNettingSettlementScript --broadcast
```

For 0G mainnet, make sure `.env` contains:

```shell
PRIVATE_KEY=0x...
ETH_GAS_PRICE=3000000000
ETH_PRIORITY_GAS_PRICE=2000000001
TOKEN_ADDRESS=0x...
NETTING_SETTLER_ADDRESS=0x...
```

### Verify on 0G Chain Scan

0G mainnet uses a custom verifier endpoint:

```shell
https://chainscan.0g.ai/open/api
```

Make sure `.env` contains `ETHERSCAN_API_KEY`. 0G's docs use a placeholder value, so `ETHERSCAN_API_KEY=PLACEHOLDER` is enough if you do not have a real key.

Do not use `--watch` with 0G's custom verifier. The submission can succeed, but Foundry may fail when polling the verification status.

Verify `RiveUSD`:

```shell
FOUNDRY_PROFILE=0g_mainnet forge verify-contract \
  --chain-id 16661 \
  --verifier custom \
  --verifier-api-key "${ETHERSCAN_API_KEY:-PLACEHOLDER}" \
  --verifier-url "https://chainscan.0g.ai/open/api" \
  --compiler-version "v0.8.33+commit.64118f21" \
  <RIVE_USD_ADDRESS> \
  src/mocks/RiveUSD.sol:RiveUSD
```

Verify `Escrow`:

```shell
FOUNDRY_PROFILE=0g_mainnet forge verify-contract \
  --chain-id 16661 \
  --verifier custom \
  --verifier-api-key "${ETHERSCAN_API_KEY:-PLACEHOLDER}" \
  --verifier-url "https://chainscan.0g.ai/open/api" \
  --compiler-version "v0.8.33+commit.64118f21" \
  --constructor-args $(cast abi-encode "constructor(address)" <RIVE_USD_ADDRESS>) \
  <ESCROW_ADDRESS> \
  src/Escrow.sol:Escrow
```

Verify `NettingSettlement`:

```shell
FOUNDRY_PROFILE=0g_mainnet forge verify-contract \
  --chain-id 16661 \
  --verifier custom \
  --verifier-api-key "${ETHERSCAN_API_KEY:-PLACEHOLDER}" \
  --verifier-url "https://chainscan.0g.ai/open/api" \
  --compiler-version "v0.8.33+commit.64118f21" \
  --constructor-args $(cast abi-encode "constructor(address,address)" <RIVE_USD_ADDRESS> <NETTING_SETTLER_ADDRESS>) \
  <NETTING_SETTLEMENT_ADDRESS> \
  src/NettingSettlement.sol:NettingSettlement
```

### Cast

```shell
$ cast <subcommand>
```

### Help

```shell
$ forge --help
$ anvil --help
$ cast --help
```
