package constant

// MaxChannelCostRatio bounds an administrator-controlled billing multiplier.
// The limit is intentionally well below values that could make ordinary quota
// arithmetic approach the int32 persistence boundary in a single request.
const MaxChannelCostRatio = 1000.0
