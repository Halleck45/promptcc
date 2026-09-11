package analyzer

import "fmt"

// Advice turns a hot section's metrics into one actionable sentence, or ""
// when the section is not HIGH or CRITICAL. The wording depends on where
// the prompt lives: an always-loaded rules file has a different remedy than
// a prompt in code.
func Advice(m Metrics, kind string) string {
	band := BandFor(m.BranchingScore)
	if band.Label != "HIGH" && band.Label != "CRITICAL" {
		return ""
	}
	var advice string
	switch {
	case m.DecisionRatio >= 0.5 && m.Constraints == 0:
		advice = "dense conditions and no explicit rule: state the hard limits once (never / always) and drop the cases they cover"
	case kind == "rules":
		advice = "this much branching in a file loaded every session: move it into a skill that loads only when relevant"
	case kind == "skill" || kind == "agent" || kind == "command":
		advice = "candidate for a split: a decision table in a reference file, or one skill per case"
	default:
		advice = "candidate for a split: route first, then one specialized prompt per branch"
	}
	if m.InjectionChannels >= 3 {
		advice += fmt.Sprintf("; %d injection channels where one templating mechanism would do", m.InjectionChannels)
	}
	return advice
}
