package fastaitoken

const ChannelName = "fastaitoken"

// ModelList 是本渠道类型在 /api/models 里对外声明的上游模型名。
// 2.0/2.0 Fast 优先使用 time 计费版本；Mini/2.5 没有 time 模型，使用更低价的 token 版本。
// Wan 只通过 Rolldek 协议渠道接入，不属于 FastAI。
var ModelList = []string{
	"seedance-2.0-time",
	"seedance-2.0-fast-time",
	"seedance-2.0-mini-token",
	"seedance-2.5-token",
}

const (
	// SubmitEndpoint and FetchEndpoint follow the source FastAI Seedance contract.
	// 站点同时提供 /v1/videos（Sora 形态）指向同一 handler，这里固定用带
	// /generations 的一组，与提交路径同族。
	SubmitEndpoint = "/v1/videos/generations"
	FetchEndpoint  = "/v1/videos/generations/%s"
)

// Status words from source fixtures; this port has not made paid provider calls.
const (
	StatusQueued     = "queued"
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusRunning    = "running"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
	StatusCancelled  = "cancelled"
)

// ResolveModel separates the public customer model from the upstream protocol and unit.
func ResolveModel(name string) (upstream, unit string, ok bool) {
	switch name {
	case "doubao-seedance-2-5-260628", "seedance-2.5-token":
		return "seedance-2.5-token", "tokens", true
	case "doubao-seedance-2-0-mini-260615", "seedance-2.0-mini-token":
		return "seedance-2.0-mini-token", "tokens", true
	case "doubao-seedance-2-0-260128", "seedance-2.0-time":
		return "seedance-2.0-time", "seconds", true
	case "doubao-seedance-2-0-fast-260128", "seedance-2.0-fast-time":
		return "seedance-2.0-fast-time", "seconds", true
	default:
		return "", "", false
	}
}
