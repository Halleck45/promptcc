// Package analyzer computes complexity metrics for LLM prompts.
//
// The core idea, borrowed from research on LLM-integrated applications, is
// that prompt length predicts nothing: what predicts maintenance pain is
// branching, i.e. the number of distinct decisions, routing points and
// coupling channels encoded in the prompt. Volume metrics are still reported
// as a control, precisely to show they do not correlate.
package analyzer

import (
	"regexp"
	"strings"
)

// Metrics holds the analysis result for one prompt.
type Metrics struct {
	Name string `json:"name"`

	// Volume: reported as a control group, does not predict maintenance pain.
	Chars        int `json:"n_chars"`
	Words        int `json:"n_words"`
	Instructions int `json:"n_instructions"`

	// Branching: the signals that matter. Distinct things, not volume.
	Decisions     int     `json:"n_decisions"`
	DecisionRatio float64 `json:"p_dec_ratio"`
	Constraints   int     `json:"n_constraints"`
	Roles         int     `json:"n_roles"`
	ToolRoutes    int     `json:"n_tool_routes"`

	// Injection surface: coupling channels between code and prompt.
	InjectionChannels int `json:"inject_surf"`
	InjectionSlots    int `json:"n_inject_slots"`

	// Output contract.
	OutputStructured bool `json:"output_structured"`
	OutputDepth      int  `json:"output_depth"`

	// Composite score.
	BranchingScore float64 `json:"branching_score"`

	Detail Detail `json:"detail"`
}

// Detail exposes the evidence behind the counters.
type Detail struct {
	DecisionsByKeyword  map[string]int `json:"decisions_by_keyword"`
	InjectionByChannel  map[string]int `json:"injection_by_channel"`
	ConditionalUnits    int            `json:"conditional_instruction_units"`
	InstructionUnitKind string         `json:"instruction_unit_kind"`
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// Analyze computes metrics for the given prompt text.
func Analyze(text, name string) Metrics {
	m := Metrics{Name: name}
	m.Chars = len([]rune(text))
	m.Words = len(wordRe.FindAllString(text, -1))

	units, kind := instructionUnits(text)
	m.Instructions = max(len(units), 1)

	tokens := tokenize(text)
	var decDetail map[string]int
	m.Decisions, decDetail = countMatches(tokens, decisionPhrases)
	m.Constraints, _ = countMatches(tokens, constraintPhrases)
	m.Roles, _ = countMatches(tokens, rolePhrases)
	m.ToolRoutes, _ = countMatches(tokens, toolRoutePhrases)

	conditional := 0
	for _, u := range units {
		if containsAny(tokenize(u), decisionPhrases) {
			conditional++
		}
	}
	m.DecisionRatio = round3(float64(conditional) / float64(m.Instructions))

	m.InjectionChannels, m.InjectionSlots, m.Detail.InjectionByChannel = countInjections(text)

	outputHits, _ := countMatches(tokens, outputHintPhrases)
	m.OutputStructured = outputHits > 0
	m.OutputDepth = maxBraceDepth(text)

	m.BranchingScore = branchingScore(&m)
	m.Detail.DecisionsByKeyword = decDetail
	m.Detail.ConditionalUnits = conditional
	m.Detail.InstructionUnitKind = kind
	return m
}

// instructionUnits splits a prompt into instruction units. Prompts written as
// bullet lists split naturally on lines; prompts written in prose split on
// sentences. We take whichever segmentation yields more units so that prose
// prompts are not undercounted as a single instruction.
func instructionUnits(text string) (units []string, kind string) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.Trim(l, " \t-*•>")
		if len([]rune(l)) > 3 {
			lines = append(lines, l)
		}
	}
	var sentences []string
	for _, s := range sentenceRe.Split(text, -1) {
		s = strings.TrimSpace(s)
		if len([]rune(s)) > 3 {
			sentences = append(sentences, s)
		}
	}
	if len(sentences) > len(lines) {
		return sentences, "sentences"
	}
	return lines, "lines"
}

var sentenceRe = regexp.MustCompile(`(?:[.!?])\s+`)

func round3(f float64) float64 {
	return float64(int(f*1000+0.5)) / 1000
}
