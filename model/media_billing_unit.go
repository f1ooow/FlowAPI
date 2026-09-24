package model

import (
	_ "embed"

	"github.com/QuantumNous/new-api/common"
)

// The same contract catalog is imported by the frontend pricing editor. It
// contains units only; it never installs prices or enables providers.
//
//go:embed media_billing_units.json
var mediaBillingUnitsJSON []byte

type mediaBillingUnit struct {
	Fixed string `json:"fixed"`
	Ratio string `json:"ratio"`
	Task  bool   `json:"task"`
}

var mediaBillingUnits = func() map[string]mediaBillingUnit {
	var units map[string]mediaBillingUnit
	if err := common.Unmarshal(mediaBillingUnitsJSON, &units); err != nil {
		panic(err)
	}
	return units
}()

// MediaBillingUnit reports the quantity charged by the selected pricing mode.
// Async tasks use ModelPriceHelperPerCall and do not evaluate token expressions.
func MediaBillingUnit(modelName string, usePrice bool, billingMode string) string {
	contract, known := mediaBillingUnits[modelName]
	if billingMode == "tiered_expr" && !contract.Task {
		return "expression"
	}
	if usePrice {
		if known {
			return contract.Fixed
		}
		return "request"
	}
	if contract.Ratio != "" {
		return contract.Ratio
	}
	return "tokens"
}
