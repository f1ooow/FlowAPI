package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaBillingUnitKeepsPerCallMiniMaxSeparateFromCharacters(t *testing.T) {
	require.Equal(t, "request", MediaBillingUnit("minimax-voice-clone", true, ""))
	require.Equal(t, "characters", MediaBillingUnit("doubao-tts-2.0", false, ""))
}

func TestMediaBillingUnitSeparatesFastAITimeAndTokenAliases(t *testing.T) {
	require.Equal(t, "seconds", MediaBillingUnit("seedance-2.0-time", true, ""))
	require.Equal(t, "tokens", MediaBillingUnit("seedance-2.5-token", false, ""))
}

func TestMediaBillingUnitExpressionDoesNotOverrideTaskContract(t *testing.T) {
	require.Equal(t, "seconds", MediaBillingUnit("happyhorse-1.1-t2v", true, "tiered_expr"))
	require.Equal(t, "expression", MediaBillingUnit("text-model", false, "tiered_expr"))
}
