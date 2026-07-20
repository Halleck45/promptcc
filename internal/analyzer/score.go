package analyzer

import "math"

// Score weights. These are a documented v0 heuristic, not calibrated
// coefficients: the relative ordering follows the correlations reported for
// LLM-integrated applications (decision density and routing dominate,
// explicit guardrails relieve), but the absolute values are a judgment call.
// Calibration against a labeled corpus is tracked on the roadmap.
const (
	weightDecision   = 1.0
	weightRatio      = 10.0
	weightToolRoute  = 1.5
	weightInjection  = 1.0
	weightDepth      = 0.5
	guardrailRelief  = 0.3 // score relief per explicit guardrail
	maxReliefPercent = 0.4 // guardrails can relieve at most 40% of the raw score
)

// Band is a human-readable severity band for a branching score.
type Band struct {
	Label string
	Hint  string
}

func branchingScore(m *Metrics) float64 {
	raw := float64(m.Decisions)*weightDecision +
		m.DecisionRatio*weightRatio +
		float64(m.ToolRoutes)*weightToolRoute +
		float64(m.InjectionChannels)*weightInjection +
		float64(m.OutputDepth)*weightDepth
	relief := math.Min(float64(m.Constraints)*guardrailRelief, raw*maxReliefPercent)
	return math.Round(math.Max(0, raw-relief)*100) / 100
}

// BandFor maps a branching score to a severity band.
func BandFor(score float64) Band {
	switch {
	case score < 5:
		return Band{"LOW", "linear prompt, little branching"}
	case score < 12:
		return Band{"MODERATE", "a few decisions to keep consistent"}
	case score < 22:
		return Band{"HIGH", "notable decision density, refactor candidate"}
	default:
		return Band{"CRITICAL", "dense business logic hidden in the prompt"}
	}
}
