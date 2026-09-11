package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedirectRulesMatchFirstOrderedRule(t *testing.T) {
	rules := []ModelRedirectRule{
		{MatchType: ModelRedirectMatchContains, Source: "opus", Target: "claude-opus-4-6"},
		{MatchType: ModelRedirectMatchRegex, Source: "^claude-.*-5$", Target: "fallback"},
	}
	target, matched := MatchModelRedirect(rules, "claude-opus-5")
	require.True(t, matched)
	assert.Equal(t, "claude-opus-4-6", target)
}

func TestModelRedirectRulesSupportAllMatchTypes(t *testing.T) {
	tests := []struct {
		matchType ModelRedirectMatchType
		source    string
		model     string
	}{
		{ModelRedirectMatchExact, "claude-opus-5", "claude-opus-5"},
		{ModelRedirectMatchPrefix, "claude-", "claude-opus-5"},
		{ModelRedirectMatchSuffix, "-5", "claude-opus-5"},
		{ModelRedirectMatchContains, "opus", "claude-opus-5"},
		{ModelRedirectMatchRegex, "^claude-(opus|sonnet)-[0-9]+$", "claude-opus-5"},
	}
	for _, tt := range tests {
		t.Run(string(tt.matchType), func(t *testing.T) {
			target, matched := MatchModelRedirect([]ModelRedirectRule{{MatchType: tt.matchType, Source: tt.source, Target: "upstream"}}, tt.model)
			require.True(t, matched)
			assert.Equal(t, "upstream", target)
		})
	}
}

func TestParseModelRedirectRulesNormalizesLegacyObjectAndWhitespace(t *testing.T) {
	rules, err := ParseModelRedirectRules(`{" z-model ":" z-upstream ","a-model":"a-upstream"}`)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, ModelRedirectRule{MatchType: ModelRedirectMatchExact, Source: "z-model", Target: "z-upstream"}, rules[1])
}

func TestValidateModelRedirectRulesRejectsInvalidRules(t *testing.T) {
	require.Error(t, ValidateModelRedirectRules([]ModelRedirectRule{{MatchType: ModelRedirectMatchRegex, Source: "(", Target: "upstream"}}))
	require.Error(t, ValidateModelRedirectRules([]ModelRedirectRule{
		{MatchType: ModelRedirectMatchContains, Source: "opus", Target: "one"},
		{MatchType: ModelRedirectMatchContains, Source: "opus", Target: "two"},
	}))
}
