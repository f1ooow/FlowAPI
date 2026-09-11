package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestClassifyRelayRetryStopsLocalAndCommittedErrors(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	localErr := types.NewError(errors.New("bad request"), types.ErrorCodeInvalidRequest)
	localDecision := ClassifyRelayRetry(c, localErr, false)
	assert.False(t, localDecision.Retryable)
	assert.False(t, localDecision.AutoBanEligible)

	upstreamErr := types.NewErrorWithStatusCode(errors.New("unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	upstreamDecision := ClassifyRelayRetry(c, upstreamErr, false)
	assert.True(t, upstreamDecision.Retryable)
	assert.True(t, upstreamDecision.AutoBanEligible)
	committedDecision := ClassifyRelayRetry(c, upstreamErr, true)
	assert.False(t, committedDecision.Retryable)
	assert.False(t, committedDecision.AutoBanEligible)
}

// TestClassifyRelayRetryFailsOverDespiteChannelAffinitySkipRetry locks the
// regression where an affinity rule with skip_retry_on_failure made every
// upstream failure terminal: groups that use affinity lost cross-channel
// failover entirely and clients got the raw 502 while healthy channels were
// still untried. Affinity is a routing preference, not a retry veto.
func TestClassifyRelayRetryFailsOverDespiteChannelAffinitySkipRetry(t *testing.T) {
	c := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName:   "rule-skip-retry",
		SkipRetry:  true,
		UsingGroup: "default",
		ModelName:  "gpt-5",
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstreamErr := types.NewErrorWithStatusCode(errors.New("bad gateway"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	decision := ClassifyRelayRetry(c, upstreamErr, false)
	assert.True(t, decision.Retryable)
	assert.True(t, decision.AutoBanEligible)
	assert.Equal(t, "retryable_upstream_status", decision.Reason)

	localErr := types.NewError(errors.New("bad request"), types.ErrorCodeInvalidRequest)
	localDecision := ClassifyRelayRetry(c, localErr, false)
	assert.False(t, localDecision.Retryable)
	assert.False(t, localDecision.AutoBanEligible)
}

func TestClassifyRelayRetryStopsClientAbort499(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	err := types.NewErrorWithStatusCode(errors.New("client aborted"), types.ErrorCodeBadResponseStatusCode, 499)
	decision := ClassifyRelayRetry(c, err, false)
	assert.False(t, decision.Retryable)
	assert.False(t, decision.AutoBanEligible)
	assert.Equal(t, "client_aborted", decision.Reason)
}
