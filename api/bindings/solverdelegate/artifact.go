package solverdelegate

import _ "embed" // embeds the upstream compiler artifact for runtime validation and local EVM tests

// Artifact is the pinned upstream compiler output. Refresh only with make refresh-solver-delegate.
// Unlike ABI calls, the payable fallback takes bytes20(target) || calldata.
//
//go:embed artifact.json
var Artifact string
