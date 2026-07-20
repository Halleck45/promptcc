package analyzer

// Lexicons are deliberately explicit lists: they are artifacts to be read,
// reviewed and contributed to. Entries are matched on whole word tokens
// (Unicode-aware, case-insensitive). A "*" skips up to 3 arbitrary tokens.
//
// English and French are built in. Adding a language means adding entries
// here, nothing else.

// decisionEntries are branching points: places where the prompt encodes a
// conditional behavior. This is the strongest maintenance-pain signal.
var decisionEntries = []string{
	// English
	"if", "else", "unless", "when", "whenever", "in case", "whether",
	"depending on", "as soon as", "as long as", "otherwise",
	// French
	"si et seulement si", "si", "sinon", "quand", "lorsque", "lorsqu'une",
	"lorsqu'un", "au cas où", "sauf si", "à moins que", "selon",
	"dans le cas", "dès que", "tant que",
}

// constraintEntries are explicit guardrails (hard obligations and
// prohibitions). They correlate negatively with maintenance pain: prompts
// that state their limits explicitly are easier to maintain.
var constraintEntries = []string{
	// English
	"never", "always", "you must", "must not", "do not", "don't",
	"mandatory", "required", "forbidden",
	// French
	"ne * jamais", "ne * pas", "ne * plus", "jamais", "toujours",
	"tu dois", "vous devez", "il faut", "impératif", "obligatoire",
	"interdit",
}

// roleEntries are role or persona definitions.
var roleEntries = []string{
	// English
	"you are", "your role", "act as", "behave as",
	// French
	"tu es", "vous êtes", "ton rôle", "votre rôle", "agis comme",
	"comporte-toi",
}

// toolRouteEntries are routing, delegation and escalation points: places
// where the prompt decides which tool, agent or channel handles the work.
var toolRouteEntries = []string{
	// English
	"call * tool", "use * tool", "hand off", "handoff", "delegate",
	"escalate", "route", "redirect",
	// French
	"utilise l'outil", "appelle l'outil", "renvoie vers", "escalade",
	"transfère", "redirige", "route",
}

// outputHintEntries signal a structured output requirement.
var outputHintEntries = []string{
	// English
	"json", "yaml", "schema", "output format", "respond in", "return only",
	// French
	"schéma", "format de sortie", "au format", "réponds en", "réponds au",
}

var (
	decisionPhrases   = compilePhrases(decisionEntries)
	constraintPhrases = compilePhrases(constraintEntries)
	rolePhrases       = compilePhrases(roleEntries)
	toolRoutePhrases  = compilePhrases(toolRouteEntries)
	outputHintPhrases = compilePhrases(outputHintEntries)
)
