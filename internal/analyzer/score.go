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

// BandInfo pairs a severity band with its lower score bound.
type BandInfo struct {
	Band
	Min float64
}

// BandScale returns the severity bands in ascending score order. Each band
// spans [Min, next.Min); the last band is open-ended.
func BandScale() []BandInfo {
	return []BandInfo{
		{Band{"LOW", "linear prompt, little branching"}, 0},
		{Band{"MODERATE", "a few decisions to keep consistent"}, 5},
		{Band{"HIGH", "notable decision density, refactor candidate"}, 12},
		{Band{"CRITICAL", "dense business logic hidden in the prompt"}, 22},
	}
}

// Component is one additive, weighted contribution to the branching score.
type Component struct {
	Key    string  // stable identifier, e.g. "decisions"
	Label  string  // human label, e.g. "decision points"
	Weight float64 // the weight applied to the underlying counter
	Value  float64 // weighted contribution, in score points
}

// Breakdown decomposes the branching score of m into its weighted
// components, the guardrail relief subtracted from their sum, and the raw
// sum itself: score = max(0, raw - relief), rounded to 2 decimals.
func Breakdown(m *Metrics) (parts []Component, relief, raw float64) {
	parts = []Component{
		{"decisions", "decision points", weightDecision, float64(m.Decisions) * weightDecision},
		{"density", "decision density", weightRatio, m.DecisionRatio * weightRatio},
		{"routes", "routing / escalation", weightToolRoute, float64(m.ToolRoutes) * weightToolRoute},
		{"injection", "injection channels", weightInjection, float64(m.InjectionChannels) * weightInjection},
		{"depth", "output schema depth", weightDepth, float64(m.OutputDepth) * weightDepth},
	}
	for _, p := range parts {
		raw += p.Value
	}
	relief = math.Min(float64(m.Constraints)*guardrailRelief, raw*maxReliefPercent)
	return parts, relief, raw
}

func branchingScore(m *Metrics) float64 {
	_, relief, raw := Breakdown(m)
	return math.Round(math.Max(0, raw-relief)*100) / 100
}

// BandFor maps a branching score to a severity band.
func BandFor(score float64) Band {
	scale := BandScale()
	band := scale[0].Band
	for _, b := range scale {
		if score >= b.Min {
			band = b.Band
		}
	}
	return band
}
