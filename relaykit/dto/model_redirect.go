package dto

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ModelRedirectMatchType string

const (
	ModelRedirectMatchExact    ModelRedirectMatchType = "exact"
	ModelRedirectMatchPrefix   ModelRedirectMatchType = "prefix"
	ModelRedirectMatchSuffix   ModelRedirectMatchType = "suffix"
	ModelRedirectMatchContains ModelRedirectMatchType = "contains"
	ModelRedirectMatchRegex    ModelRedirectMatchType = "regex"

	MaxModelRedirectRules        = 100
	MaxModelRedirectSourceLength = 512
	MaxModelRedirectTargetLength = 255
)

type ModelRedirectRule struct {
	MatchType ModelRedirectMatchType `json:"match_type"`
	Source    string                 `json:"source"`
	Target    string                 `json:"target"`
}

func ParseModelRedirectRules(raw string) ([]ModelRedirectRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "[]" || raw == "null" {
		return nil, nil
	}

	var rules []ModelRedirectRule
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return nil, fmt.Errorf("invalid model redirect rules: %w", err)
		}
	} else {
		legacy := make(map[string]string)
		if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
			return nil, fmt.Errorf("invalid legacy model mapping: %w", err)
		}
		normalizedLegacy := make(map[string]string, len(legacy))
		for source, target := range legacy {
			source = strings.TrimSpace(source)
			if _, exists := normalizedLegacy[source]; exists {
				return nil, fmt.Errorf("legacy model mapping contains duplicate source after normalization: %s", source)
			}
			normalizedLegacy[source] = strings.TrimSpace(target)
		}
		keys := make([]string, 0, len(normalizedLegacy))
		for source := range normalizedLegacy {
			keys = append(keys, source)
		}
		sort.Strings(keys)
		for _, source := range keys {
			rules = append(rules, ModelRedirectRule{MatchType: ModelRedirectMatchExact, Source: source, Target: normalizedLegacy[source]})
		}
	}

	for i := range rules {
		rules[i].MatchType = ModelRedirectMatchType(strings.ToLower(strings.TrimSpace(string(rules[i].MatchType))))
		rules[i].Source = strings.TrimSpace(rules[i].Source)
		rules[i].Target = strings.TrimSpace(rules[i].Target)
	}
	if err := ValidateModelRedirectRules(rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func ValidateModelRedirectRules(rules []ModelRedirectRule) error {
	if len(rules) > MaxModelRedirectRules {
		return fmt.Errorf("model redirect rules cannot exceed %d", MaxModelRedirectRules)
	}
	seen := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		source := strings.TrimSpace(rule.Source)
		target := strings.TrimSpace(rule.Target)
		if source == "" || target == "" {
			return fmt.Errorf("model redirect rule %d requires source and target", i)
		}
		if len(source) > MaxModelRedirectSourceLength || len(target) > MaxModelRedirectTargetLength {
			return fmt.Errorf("model redirect rule %d exceeds length limit", i)
		}
		switch rule.MatchType {
		case ModelRedirectMatchExact, ModelRedirectMatchPrefix, ModelRedirectMatchSuffix, ModelRedirectMatchContains:
		case ModelRedirectMatchRegex:
			if _, err := regexp.Compile(source); err != nil {
				return fmt.Errorf("model redirect rule %d has invalid regex: %w", i, err)
			}
		default:
			return fmt.Errorf("model redirect rule %d has invalid match_type: %s", i, rule.MatchType)
		}
		key := string(rule.MatchType) + "\x00" + source
		if _, ok := seen[key]; ok {
			return fmt.Errorf("model redirect rule %d duplicates an earlier rule", i)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func MatchModelRedirect(rules []ModelRedirectRule, model string) (string, bool) {
	for _, rule := range rules {
		matched := false
		switch rule.MatchType {
		case ModelRedirectMatchExact:
			matched = model == rule.Source
		case ModelRedirectMatchPrefix:
			matched = strings.HasPrefix(model, rule.Source)
		case ModelRedirectMatchSuffix:
			matched = strings.HasSuffix(model, rule.Source)
		case ModelRedirectMatchContains:
			matched = strings.Contains(model, rule.Source)
		case ModelRedirectMatchRegex:
			re, err := regexp.Compile(rule.Source)
			matched = err == nil && re.MatchString(model)
		}
		if matched {
			return rule.Target, true
		}
	}
	return "", false
}
