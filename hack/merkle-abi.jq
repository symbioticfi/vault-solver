# Solver-facing projection of canonical Foundry artifacts; keep complete tuple types.
.abi | map(select(
  if $contract == "LiquidLaneConnector" then
    .type == "function" and (
      .name == "account" or .name == "outputAsset" or .name == "remainingAssets" or
      (.name == "swap" and (.inputs | length) == 4))
  elif $contract == "IThreeFPolicy" then
    (.type == "event" and .name == "Consume") or
    (.type == "function" and (.name == "account" or .name == "remainingAssets" or
      .name == "minYieldPerRequest" or .name == "consume"))
  elif $contract == "IRecipeCapability" then .type == "function" and .name == "execute"
  else error("unknown Merkle ABI projection")
  end
))
