// Vendored verbatim from RedStone (received via direct chat, 2026-06-12).
// Contract-of-record for the RedStone Atom OEV WebSocket messages, per the repo's
// vendor-the-source rule (CLAUDE.md "Code generation"). Not built or executed here —
// the Go structs in internal/solvers/redstoneoev/ are pinned to this file by tests.
//
// KNOWN GAP: RedStone has not (yet) shared the schema of the inbound auction broadcast
// (`op: "auction"`, incl. the liquidations-mode positions/prices payload). Until they do,
// that frame's contract-of-record is the docs example + live frames captured in P0
// (see docs/OEV-PLAN.md §6.1, §6.3) — formalized in openapi/redstone-oev.asyncapi.yaml.
//
// Fields RedStone's schema adds beyond the public docs:
//   solve.data.borrowers?: string[]   — semantics unconfirmed (asked; likely telemetry/validation)
//   solve.data.profit?: string        — semantics unconfirmed (asked)
//   liquidation-result.data.error?: string

const WsMessageSubSchema = z.object({
  op: z.literal('subscribe'),
  topic: z.string(),
});

const WsMessageUnSubSchema = z.object({
  op: z.literal('unsubscribe'),
  topic: z.string(),
});

const WsMessageSolveSchema = z.object({
  op: z.literal('solve'),
  id: z.string(),
  data: z.object({
    bid: z.string(),
    nonce: z.string(),
    operationCallback: z.string(),
    operationData: z.string(),
    liquidationSig: z.string(),
    maxTxGasPrice: z.string(),
    borrowers: z.array(z.string()).optional(),
    profit: z.string().optional(),
  }),
});

const WsMessageAuctionResult = z.object({
  op: z.literal('auction-result'),
  id: z.string(),
  data: z.object({
    bid: z.string(),
    liquidator: z.string(),
  }),
});

const WsMessageLiquidationResult = z.object({
  op: z.literal('liquidation-result'),
  id: z.string(),
  data: z.object({
    success: z.boolean(),
    txHash: z.string(),
    liquidator: z.string(),
    error: z.string().optional(),
  }),
});

const WsMessageBlacklist = z.object({
  op: z.literal('blacklisted'),
  id: z.string(),
  data: z.object({
    liquidator: z.string(),
    msg: z.string(),
  }),
});
