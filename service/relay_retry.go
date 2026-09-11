package service

import (
	"context"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type RelayRetryDecision struct {
	Retryable       bool
	AutoBanEligible bool
	Reason          string
}

func ClassifyRelayRetry(c *gin.Context, relayErr *types.NewAPIError, responseCommitted bool) RelayRetryDecision {
	if relayErr == nil {
		return RelayRetryDecision{Reason: "success"}
	}
	if responseCommitted {
		return RelayRetryDecision{Reason: "response_committed"}
	}
	if relayErr.StatusCode == 499 {
		return RelayRetryDecision{Reason: "client_aborted"}
	}
	if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		if c.Request.Context().Err() == context.Canceled {
			return RelayRetryDecision{Reason: "client_cancelled"}
		}
		return RelayRetryDecision{Reason: "request_deadline"}
	}
	if types.IsSkipRetryError(relayErr) {
		return RelayRetryDecision{Reason: "explicit_skip"}
	}
	// Channel affinity must not block cross-channel failover: a sticky rule is a
	// routing preference, not a reason to hand the client an upstream 5xx while
	// healthy channels are still untried.
	switch relayErr.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeCountTokenFailed,
		types.ErrorCodeModelPriceError,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed:
		return RelayRetryDecision{Reason: "local_or_client_error"}
	}
	if types.IsChannelError(relayErr) {
		return RelayRetryDecision{Retryable: true, AutoBanEligible: true, Reason: "channel_error"}
	}
	if relayErr.GetErrorCode() == types.ErrorCodeBadResponseBody {
		return RelayRetryDecision{Retryable: true, AutoBanEligible: true, Reason: "invalid_precommit_upstream_response"}
	}
	status := relayErr.StatusCode
	if status >= 200 && status < 300 {
		return RelayRetryDecision{Reason: "successful_status"}
	}
	if status < 100 || status > 599 {
		return RelayRetryDecision{Retryable: true, AutoBanEligible: true, Reason: "network_error"}
	}
	if status == 408 || status == 504 || status == 524 {
		return RelayRetryDecision{Retryable: true, AutoBanEligible: true, Reason: "upstream_timeout"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(relayErr.GetErrorCode()) {
		return RelayRetryDecision{Reason: "configured_skip"}
	}
	if operation_setting.ShouldRetryByStatusCode(status) {
		return RelayRetryDecision{Retryable: true, AutoBanEligible: true, Reason: "retryable_upstream_status"}
	}
	return RelayRetryDecision{Reason: "non_retryable_status"}
}
