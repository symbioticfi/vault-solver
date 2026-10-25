# Shared Merkle funding capacity

The RFQ pricing solver (`rfq-filler`) and `threef-merkle` accept the same optional
`merkleFunding` configuration inside their own `config` blocks. Entries are scoped
to an account and funding asset. Without a matching entry, sizing uses account cash.
A configured read failure stops that observation; there is no cash fallback.

```yaml
merkleFunding:
  - account: "0x1111111111111111111111111111111111111111"
    asset: "0x2222222222222222222222222222222222222222"
    lens: "0x3333333333333333333333333333333333333333"
    sources:
      - id: "0x4444444444444444444444444444444444444444444444444444444444444444"
        target: "0x5555555555555555555555555555555555555555"
        data: "0x12345678"
        offset: 0
```

These are placeholders, not deployed contracts or usable call data. Deploy
`MerkleFundingLens` from symbioticfi/adapters and replace every field with reviewed
chain-specific values. Configure matching sources in the RFQ backend deployment
manifest. No new production funding source is enabled by this change.

The lens returns account cash plus each scalar query result in funding-token base
units. Each query must match a synchronous withdrawal supported by the approved
recipe and account permissions, and preserve the recipe's health floor. Reuse its
existing capacity helpers (for example AaveV3Liquidity.liquidityWithBorrow or
MorphoLiquidity.liquidity), or a compatible vault's maxWithdraw(account). All calls
use the quote or recipe snapshot block. Queries requiring preparation writes need
an equivalent reviewed view helper.

Source ids identify canonical liquidity venues/positions. Duplicate ids and
identical queries are rejected. Sources must be disjoint: do not combine alternate
exits from one position, a vault with its underlying position, overlapping pool
cash, or cash already counted by balanceOf(account). The lens cannot infer arbitrary
overlap, ownership or permissions from calldata. The disjointness requirement also
applies across accounts/routes combined into one quote. Configuration is an operator trust
boundary, not data accepted from the RFQ request or 3F API.

LL quotes remain capped by the connector budget and advertised backend capacity;
RFQ reservations still share the account/asset capacity identity. 3F offer sizing
uses min(pair budget minus pair reservations, funding minus asset reservations).
3F configuration changes participate in recovery identity. Independent RFQ and 3F
workflows can still compete for the same live funds; these reads do not reserve
funds on chain, and the actual fill/mint must pass final execution simulation.

At most 64 account/asset pairs and 32 sources per pair are accepted. `offset` is a
32-byte-aligned byte offset, at most 65536. Calldata has at least a selector and at
most 8192 bytes. No NAV, staged adapter cash or asynchronous exit value is counted.
