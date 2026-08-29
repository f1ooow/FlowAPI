package constant

// MinChannelCostRatio and MaxChannelCostRatio bound an administrator-controlled
// billing multiplier. Values are stored and edited to hundredth precision.
// The limit is intentionally well below values that could make ordinary quota
// arithmetic approach the int32 persistence boundary in a single request.
const MinChannelCostRatio = 0.01

const MaxChannelCostRatio = 1000.0
