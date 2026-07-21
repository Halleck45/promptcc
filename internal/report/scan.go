package report

import (
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/halleck45/promptcc/internal/analyzer"
)

// ScanEntry is one analyzed prompt found by promptcc scan.
type ScanEntry struct {
	File       string           `json:"file"`
	Line       int              `json:"line"`
	EndLine    int              `json:"end_line"`
	Confidence string           `json:"confidence"`
	Context    string           `json:"context"`
	Slots      []string         `json:"slots,omitempty"`
	Text       string           `json:"-"`
	Metrics    analyzer.Metrics `json:"metrics"`
}

// HTML renders a self-contained HTML report for a scan.
func HTML(entries []ScanEntry, version string) (string, error) {
	sorted := make([]ScanEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Metrics.BranchingScore > sorted[j].Metrics.BranchingScore
	})

	type row struct {
		ScanEntry
		Band      analyzer.Band
		BandClass string
		Keywords  []kv
		Channels  []kv
	}
	rows := make([]row, 0, len(sorted))
	bandCounts := map[string]int{}
	files := map[string]bool{}
	for _, e := range sorted {
		band := analyzer.BandFor(e.Metrics.BranchingScore)
		bandCounts[band.Label]++
		files[e.File] = true
		rows = append(rows, row{
			ScanEntry: e,
			Band:      band,
			BandClass: strings.ToLower(band.Label),
			Keywords:  sortedByCount(e.Metrics.Detail.DecisionsByKeyword),
			Channels:  sortedByCount(e.Metrics.Detail.InjectionByChannel),
		})
	}

	data := struct {
		Rows       []row
		BandCounts map[string]int
		Bands      []string
		Files      int
		Version    string
		Date       string
	}{
		Rows:       rows,
		BandCounts: bandCounts,
		Bands:      []string{"CRITICAL", "HIGH", "MODERATE", "LOW"},
		Files:      len(files),
		Version:    version,
		Date:       time.Now().Format("2006-01-02 15:04"),
	}

	var b strings.Builder
	if err := htmlTemplate.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"index0": func(m map[string]int, k string) int { return m[k] },
	"lower":  strings.ToLower,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>promptcc report</title>
<style>
  :root {
    --bg: #f6f7f9; --card: #ffffff; --text: #1c2430; --muted: #617082;
    --border: #dfe4ea;
    --critical: #c0392b; --high: #d97706; --moderate: #b0900f; --low: #2f8a4c;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #14181e; --card: #1d232c; --text: #e6ebf2; --muted: #93a1b5;
      --border: #303a47;
      --critical: #e06b5d; --high: #e8a04c; --moderate: #cfb04a; --low: #5bb47e;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 2rem 1rem; background: var(--bg); color: var(--text);
    font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
  }
  main { max-width: 60rem; margin: 0 auto; }
  h1 { font-size: 1.4rem; margin: 0; }
  h1 span { color: var(--muted); font-weight: normal; }
  .meta { color: var(--muted); margin: .25rem 0 1.25rem; }
  .chips { display: flex; flex-wrap: wrap; gap: .5rem; margin-bottom: 1.5rem; }
  .chip {
    padding: .25rem .7rem; border-radius: 99px; font-size: .85rem;
    border: 1px solid var(--border); background: var(--card);
  }
  .chip b { font-variant-numeric: tabular-nums; }
  .chip.critical b { color: var(--critical); } .chip.high b { color: var(--high); }
  .chip.moderate b { color: var(--moderate); } .chip.low b { color: var(--low); }
  details.prompt {
    background: var(--card); border: 1px solid var(--border);
    border-left: 4px solid var(--border); border-radius: 8px;
    margin-bottom: .6rem; overflow: hidden;
  }
  details.prompt.critical { border-left-color: var(--critical); }
  details.prompt.high     { border-left-color: var(--high); }
  details.prompt.moderate { border-left-color: var(--moderate); }
  details.prompt.low      { border-left-color: var(--low); }
  summary {
    display: flex; flex-wrap: wrap; gap: .35rem 1rem; align-items: baseline;
    padding: .7rem 1rem; cursor: pointer; list-style: none;
  }
  summary::-webkit-details-marker { display: none; }
  .score { font-weight: 700; font-variant-numeric: tabular-nums; min-width: 3.5rem; }
  .band { font-size: .75rem; font-weight: 700; letter-spacing: .04em; }
  .critical .band { color: var(--critical); } .high .band { color: var(--high); }
  .moderate .band { color: var(--moderate); } .low .band { color: var(--low); }
  .loc { font-family: ui-monospace, monospace; font-size: .85rem; word-break: break-all; }
  .ctx { color: var(--muted); font-size: .85rem; }
  .body { padding: 0 1rem 1rem; border-top: 1px solid var(--border); }
  .grid {
    display: grid; grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
    gap: .5rem; margin: 1rem 0;
  }
  .metric { background: var(--bg); border-radius: 6px; padding: .5rem .7rem; }
  .metric .v { font-size: 1.15rem; font-weight: 700; font-variant-numeric: tabular-nums; }
  .metric .k { color: var(--muted); font-size: .78rem; }
  .lists { display: flex; flex-wrap: wrap; gap: 2rem; margin-bottom: 1rem; }
  .lists h3 { font-size: .8rem; text-transform: uppercase; letter-spacing: .05em;
    color: var(--muted); margin: 0 0 .3rem; }
  .lists ul { margin: 0; padding: 0; list-style: none; font-size: .85rem; }
  .lists li b { font-variant-numeric: tabular-nums; }
  pre.text {
    background: var(--bg); border: 1px solid var(--border); border-radius: 6px;
    padding: .8rem; font-size: .82rem; white-space: pre-wrap; word-break: break-word;
    max-height: 22rem; overflow: auto; margin: 0;
  }
  footer { color: var(--muted); font-size: .8rem; margin-top: 2rem; }
</style>
</head>
<body>
<main>
  <h1>promptcc <span>· branching complexity report</span></h1>
  <p class="meta">{{len .Rows}} prompt(s) in {{.Files}} file(s) · generated {{.Date}}</p>
  <div class="chips">
    {{- range .Bands}}
    <span class="chip {{lower .}}"><b>{{index0 $.BandCounts .}}</b> {{.}}</span>
    {{- end}}
  </div>

  {{- range .Rows}}
  <details class="prompt {{.BandClass}}">
    <summary>
      <span class="score">{{printf "%.2f" .Metrics.BranchingScore}}</span>
      <span class="band">{{.Band.Label}}</span>
      <span class="loc">{{.File}}:{{.Line}}</span>
      <span class="ctx">{{.Context}} · confidence {{.Confidence}}</span>
    </summary>
    <div class="body">
      <div class="grid">
        <div class="metric"><div class="v">{{.Metrics.Decisions}}</div><div class="k">decision points</div></div>
        <div class="metric"><div class="v">{{.Metrics.DecisionRatio}}</div><div class="k">decision density</div></div>
        <div class="metric"><div class="v">{{.Metrics.ToolRoutes}}</div><div class="k">routing / escalation</div></div>
        <div class="metric"><div class="v">{{.Metrics.InjectionChannels}}</div><div class="k">injection channels ({{.Metrics.InjectionSlots}} slots)</div></div>
        <div class="metric"><div class="v">{{.Metrics.OutputDepth}}</div><div class="k">output schema depth</div></div>
        <div class="metric"><div class="v">{{.Metrics.Constraints}}</div><div class="k">explicit guardrails</div></div>
        <div class="metric"><div class="v">{{.Metrics.Roles}}</div><div class="k">role definitions</div></div>
        <div class="metric"><div class="v">{{.Metrics.Words}}</div><div class="k">words (control, predicts nothing)</div></div>
      </div>
      {{- if or .Keywords .Channels .Slots}}
      <div class="lists">
        {{- if .Keywords}}
        <div><h3>Decision keywords</h3><ul>
          {{- range .Keywords}}<li><b>{{.N}}×</b> {{.K}}</li>{{end -}}
        </ul></div>
        {{- end}}
        {{- if .Channels}}
        <div><h3>Injection channels</h3><ul>
          {{- range .Channels}}<li><b>{{.N}}×</b> {{.K}}</li>{{end -}}
        </ul></div>
        {{- end}}
        {{- if .Slots}}
        <div><h3>Interpolated values</h3><ul>
          {{- range .Slots}}<li>{{.}}</li>{{end -}}
        </ul></div>
        {{- end}}
      </div>
      {{- end}}
      <pre class="text">{{.Text}}</pre>
    </div>
  </details>
  {{- end}}

  <footer>promptcc {{.Version}} · branching, not volume</footer>
</main>
</body>
</html>
`))
