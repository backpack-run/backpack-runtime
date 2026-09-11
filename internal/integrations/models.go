package integrations

import (
	"fmt"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

// FilterCodingModels requires the explicit "code" capability. Names, aliases,
// architecture, and runtime engine are deliberately not used as heuristics.
func FilterCodingModels(models []catalog.Model) []catalog.Model {
	result := make([]catalog.Model, 0, len(models))
	for _, model := range models {
		if hasCapability(model.Capabilities, "code") {
			result = append(result, cloneModel(model))
		}
	}
	return result
}

func ModelSupports(descriptor Descriptor, model catalog.Model) bool {
	return hasCapability(model.Capabilities, descriptor.RequiredModelCapability)
}

func hasCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if strings.EqualFold(strings.TrimSpace(capability), strings.TrimSpace(required)) {
			return true
		}
	}
	return false
}

func cloneModel(model catalog.Model) catalog.Model {
	model.Aliases = append([]string(nil), model.Aliases...)
	model.ManifestIDs = append([]string(nil), model.ManifestIDs...)
	model.Capabilities = append([]string(nil), model.Capabilities...)
	return model
}

type ContextStatus string

const (
	ContextUnknown          ContextStatus = "unknown"
	ContextBelowRecommended ContextStatus = "below-recommended"
	ContextRecommended      ContextStatus = "recommended"
)

type ContextCheck struct {
	Status            ContextStatus
	AvailableTokens   int
	RecommendedTokens int
	Reason            string
}

func CheckContext(availableTokens, recommendedTokens int) (ContextCheck, error) {
	if availableTokens < 0 || recommendedTokens < 0 {
		return ContextCheck{}, fmt.Errorf("context token counts must not be negative")
	}
	check := ContextCheck{AvailableTokens: availableTokens, RecommendedTokens: recommendedTokens}
	if availableTokens == 0 || recommendedTokens == 0 {
		check.Status = ContextUnknown
		check.Reason = "model or integration context recommendation is not declared"
		return check, nil
	}
	if availableTokens < recommendedTokens {
		check.Status = ContextBelowRecommended
		check.Reason = fmt.Sprintf("model context %d is below the integration recommendation of %d tokens", availableTokens, recommendedTokens)
		return check, nil
	}
	check.Status = ContextRecommended
	check.Reason = "model context meets the integration recommendation"
	return check, nil
}

func CheckRecommendedContext(descriptor Descriptor, availableTokens int) (ContextCheck, error) {
	if err := descriptor.validate(); err != nil {
		return ContextCheck{}, err
	}
	return CheckContext(availableTokens, descriptor.RecommendedContextTokens)
}
