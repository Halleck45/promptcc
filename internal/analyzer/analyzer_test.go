package analyzer

import (
	"strings"
	"testing"
)

const frenchSupportPrompt = `Tu es un assistant de support client pour {company_name}.
Si le client demande un remboursement, vérifie d'abord la date d'achat.
Si l'achat date de moins de 30 jours, utilise l'outil refund_tool.
Sinon, escalade vers un agent humain.
Ne réponds jamais aux questions hors sujet.
Tu dois toujours répondre en français.
Réponds au format JSON : {"action": "...", "reason": "..."}`

func TestAnalyzeFrenchSupportPrompt(t *testing.T) {
	m := Analyze(frenchSupportPrompt, "support")

	if m.Decisions != 3 {
		t.Errorf("Decisions = %d, want 3 (si, si, sinon)", m.Decisions)
	}
	if m.ToolRoutes != 2 {
		t.Errorf("ToolRoutes = %d, want 2 (utilise l'outil, escalade)", m.ToolRoutes)
	}
	if m.Roles != 1 {
		t.Errorf("Roles = %d, want 1 (tu es)", m.Roles)
	}
	// ne * jamais, tu dois, toujours
	if m.Constraints != 3 {
		t.Errorf("Constraints = %d, want 3", m.Constraints)
	}
	// {company_name} only; the JSON example must not count.
	if m.InjectionChannels != 1 || m.InjectionSlots != 1 {
		t.Errorf("Injection = (%d, %d), want (1, 1)", m.InjectionChannels, m.InjectionSlots)
	}
	if !m.OutputStructured {
		t.Error("OutputStructured = false, want true (json + au format)")
	}
	if m.OutputDepth != 1 {
		t.Errorf("OutputDepth = %d, want 1", m.OutputDepth)
	}
	if m.Instructions != 7 {
		t.Errorf("Instructions = %d, want 7", m.Instructions)
	}
	if m.DecisionRatio <= 0.4 || m.DecisionRatio >= 0.5 {
		t.Errorf("DecisionRatio = %g, want 3/7", m.DecisionRatio)
	}
	if m.BranchingScore <= 0 {
		t.Errorf("BranchingScore = %g, want > 0", m.BranchingScore)
	}
}

func TestAnalyzeEnglishPrompt(t *testing.T) {
	prompt := `You are a triage agent.
If the ticket mentions billing, route it to the billing tool.
When the user is angry, escalate to a human.
Unless the issue is resolved, always follow up.
Never share internal notes.
Respond in JSON.`

	m := Analyze(prompt, "triage")
	if m.Decisions != 3 {
		t.Errorf("Decisions = %d, want 3 (if, when, unless)", m.Decisions)
	}
	if m.ToolRoutes != 2 {
		t.Errorf("ToolRoutes = %d, want 2 (route, escalate)", m.ToolRoutes)
	}
	if m.Constraints != 2 {
		t.Errorf("Constraints = %d, want 2 (always, never)", m.Constraints)
	}
	if m.Roles != 1 {
		t.Errorf("Roles = %d, want 1", m.Roles)
	}
	if !m.OutputStructured {
		t.Error("OutputStructured = false, want true")
	}
}

func TestAnalyzeEmptyInput(t *testing.T) {
	m := Analyze("", "empty")
	if m.BranchingScore != 0 {
		t.Errorf("BranchingScore = %g, want 0", m.BranchingScore)
	}
	if m.Instructions != 1 {
		t.Errorf("Instructions = %d, want 1 (floor)", m.Instructions)
	}
	if m.DecisionRatio != 0 {
		t.Errorf("DecisionRatio = %g, want 0", m.DecisionRatio)
	}
}

func TestAnalyzeProseUsesSentences(t *testing.T) {
	prose := "You are a helper. If asked about pricing, defer to sales. " +
		"When the user is done, say goodbye. Otherwise keep helping."
	m := Analyze(prose, "prose")
	if m.Detail.InstructionUnitKind != "sentences" {
		t.Errorf("InstructionUnitKind = %q, want sentences", m.Detail.InstructionUnitKind)
	}
	if m.Instructions != 4 {
		t.Errorf("Instructions = %d, want 4 sentences", m.Instructions)
	}
	if m.Detail.ConditionalUnits != 3 {
		t.Errorf("ConditionalUnits = %d, want 3 (if, when, otherwise)", m.Detail.ConditionalUnits)
	}
}

func TestAnalyzeVolumeIsIndependentFromScore(t *testing.T) {
	// A long linear prompt must score lower than a short branchy one.
	linear := strings.Repeat("Describe the product accurately and cite sources. ", 40)
	branchy := "If A, use tool X. If B, escalate. Unless C, route to D. Depending on E, hand off."

	ml := Analyze(linear, "linear")
	mb := Analyze(branchy, "branchy")
	if ml.Words < mb.Words {
		t.Fatal("test setup broken: linear prompt should be longer")
	}
	if ml.BranchingScore >= mb.BranchingScore {
		t.Errorf("linear score %g >= branchy score %g: volume is leaking into the score",
			ml.BranchingScore, mb.BranchingScore)
	}
}

func TestGuardrailsLowerTheScore(t *testing.T) {
	base := "If A, do X. If B, do Y. When C, escalate."
	guarded := base + " Never invent data. Always cite sources. Do not reply off topic."
	ms := Analyze(base, "base")
	mg := Analyze(guarded, "guarded")
	if mg.BranchingScore >= ms.BranchingScore {
		t.Errorf("guarded score %g >= base score %g: guardrails should relieve",
			mg.BranchingScore, ms.BranchingScore)
	}
}
