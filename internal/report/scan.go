package report

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/halleck45/promptcc/internal/analyzer"
)

//go:embed assets/logo-icon.png
var logoPNG []byte

// logoDataURI is the promptcc bubble icon as an inline data URI, so the report
// stays a single self-contained file.
var logoDataURI = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG))

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

// distBins is the target number of histogram bins in the score distribution.
const distBins = 36

type card struct {
	Value   string
	Label   string
	Points  string // contribution to the score, e.g. "+8.0 pts"
	Dim     bool   // zero value: de-emphasized
	Relief  bool   // guardrails: subtracts from the score
	Control bool   // volume metrics: control group
}

type codeLine struct {
	No   int
	Text string
}

type bin struct {
	LeftPct   string
	WidthPct  string
	HeightPct string
	Class     string
	Title     string
}

type bandRegion struct {
	Class    string
	Label    string
	Range    string
	Count    int
	PctStr   string // share of all prompts, e.g. "31"
	WidthPct string // segment width for the breakdown bar (0 if empty)
	Hint     string
}

type tick struct {
	LeftPct string
	Label   string
}

func bandClass(label string) string { return strings.ToLower(label) }

func fpct(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

func fscore(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func fpts(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64) }

// shortDir keeps at most the last two directories of a path, for display.
func shortDir(dir string) string {
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return ""
	}
	parts := strings.Split(dir, "/")
	if len(parts) <= 2 {
		return dir + "/"
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/") + "/"
}

// codeLines splits prompt text into numbered lines for the code block.
func codeLines(text string) []codeLine {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	lines := strings.Split(text, "\n")
	out := make([]codeLine, len(lines))
	for i, l := range lines {
		out[i] = codeLine{No: i + 1, Text: l}
	}
	return out
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := min(max(int(math.Ceil(q*float64(n)))-1, 0), n-1)
	return sorted[idx]
}

// HTML renders a self-contained HTML report for a scan.
func HTML(entries []ScanEntry, version string) (string, error) {
	sorted := make([]ScanEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Metrics.BranchingScore > sorted[j].Metrics.BranchingScore
	})

	scores := make([]float64, 0, len(sorted))
	files := map[string]bool{}
	for _, e := range sorted {
		scores = append(scores, e.Metrics.BranchingScore)
		files[e.File] = true
	}
	asc := make([]float64, len(scores))
	copy(asc, scores)
	sort.Float64s(asc)

	// Aggregate stats are computed over prompts that carry any branching at
	// all: builder classes produce many inert fragments (score 0) that would
	// otherwise pin the median to zero and hide the real signal.
	scored := make([]float64, 0, len(asc))
	for _, s := range asc {
		if s > 0 {
			scored = append(scored, s)
		}
	}
	inert := len(asc) - len(scored)

	med := median(scored)
	p90 := quantile(scored, 0.9)
	maxScore := 0.0
	if n := len(asc); n > 0 {
		maxScore = asc[n-1]
	}
	scale := analyzer.BandScale()
	axisMax := math.Max(maxScore*1.05, scale[len(scale)-1].Min+2)

	type row struct {
		ScanEntry
		Band      analyzer.Band
		BandClass string
		LowConf   bool
		Dir, Base string
		Ctx       string // display context, blank for internal heuristic markers
		ScoreStr  string
		Cards     []card
		Channels  []kv
		Code      []codeLine
	}

	bandCounts := map[string]int{}
	lowConf := 0
	totalLines := 0
	rows := make([]row, 0, len(sorted))
	for _, e := range sorted {
		m := e.Metrics
		band := analyzer.BandFor(m.BranchingScore)
		bandCounts[band.Label]++
		if e.Confidence == "low" {
			lowConf++
		}
		dir, base := path.Split(e.File)

		// The "heuristic: ..." context is an internal marker for text-only
		// hits, not a useful location like a variable name or call site.
		ctx := e.Context
		if strings.HasPrefix(ctx, "heuristic") {
			ctx = ""
		}

		code := codeLines(e.Text)
		totalLines += len(code)

		parts, relief, _ := analyzer.Breakdown(&m)
		pts := func(v float64) string {
			if v <= 0 {
				return ""
			}
			return "+" + fpts(v) + " pts"
		}
		reliefPts := ""
		if relief > 0 {
			reliefPts = "−" + fpts(relief) + " pts"
		}
		cards := []card{
			{Value: strconv.Itoa(m.Decisions), Label: "decision points", Points: pts(parts[0].Value), Dim: m.Decisions == 0},
			{Value: strconv.FormatFloat(m.DecisionRatio, 'g', -1, 64), Label: "decision density", Points: pts(parts[1].Value), Dim: m.DecisionRatio == 0},
			{Value: strconv.Itoa(m.ToolRoutes), Label: "routing / escalation", Points: pts(parts[2].Value), Dim: m.ToolRoutes == 0},
			{Value: strconv.Itoa(m.InjectionChannels), Label: fmt.Sprintf("injection channels (%d slots)", m.InjectionSlots), Points: pts(parts[3].Value), Dim: m.InjectionChannels == 0},
			{Value: strconv.Itoa(m.OutputDepth), Label: "output schema depth", Points: pts(parts[4].Value), Dim: m.OutputDepth == 0},
			{Value: strconv.Itoa(m.Constraints), Label: "explicit guardrails", Points: reliefPts, Relief: true, Dim: m.Constraints == 0},
			{Value: strconv.Itoa(m.Roles), Label: "role definitions", Dim: m.Roles == 0},
			{Value: strconv.Itoa(m.Words), Label: "words (control, predicts nothing)", Control: true},
		}

		rows = append(rows, row{
			ScanEntry: e,
			Band:      band,
			BandClass: bandClass(band.Label),
			LowConf:   e.Confidence == "low",
			Dir:       shortDir(dir),
			Base:      base,
			Ctx:       ctx,
			ScoreStr:  fscore(m.BranchingScore),
			Cards:     cards,
			Channels:  sortedByCount(m.Detail.InjectionByChannel),
			Code:      code,
		})
	}

	// Histogram bins over [0, axisMax], edges aligned on band thresholds so
	// no bin straddles a boundary and each bar carries its band's exact color.
	var edges []float64
	for i, b := range scale {
		next := axisMax
		if i+1 < len(scale) {
			next = scale[i+1].Min
		}
		n := max(1, int(math.Round(float64(distBins)*(next-b.Min)/axisMax)))
		step := (next - b.Min) / float64(n)
		for k := range n {
			edges = append(edges, b.Min+float64(k)*step)
		}
	}
	edges = append(edges, axisMax)

	counts := make([]int, len(edges)-1)
	maxCount := 0
	for _, s := range scores {
		j := min(max(sort.SearchFloat64s(edges, s+1e-9)-1, 0), len(counts)-1)
		counts[j]++
		if counts[j] > maxCount {
			maxCount = counts[j]
		}
	}
	var bins []bin
	for j, c := range counts {
		if c == 0 {
			continue
		}
		bins = append(bins, bin{
			LeftPct:   fpct(100 * edges[j] / axisMax),
			WidthPct:  fpct(100 * (edges[j+1] - edges[j]) / axisMax),
			HeightPct: fpct(math.Max(6, 100*float64(c)/float64(maxCount))),
			Class:     bandClass(analyzer.BandFor(edges[j]).Label),
			Title:     fmt.Sprintf("score %.1f to %.1f: %d prompt(s)", edges[j], edges[j+1], c),
		})
	}

	// Severity bands: legend rows, breakdown-bar segments, axis ticks, seps.
	total := len(rows)
	regions := make([]bandRegion, 0, len(scale))
	ticks := []tick{{"0", "0"}}
	var seps []string
	for i, b := range scale {
		next := axisMax
		if i+1 < len(scale) {
			next = scale[i+1].Min
		}
		rng := ""
		switch {
		case i == 0:
			rng = fmt.Sprintf("score < %g", next)
		case i == len(scale)-1:
			rng = fmt.Sprintf("score ≥ %g", b.Min)
		default:
			rng = fmt.Sprintf("%g ≤ score < %g", b.Min, next)
		}
		c := bandCounts[b.Label]
		pct, width := 0.0, "0"
		if total > 0 {
			pct = 100 * float64(c) / float64(total)
			if c > 0 {
				width = fpct(pct)
			}
		}
		regions = append(regions, bandRegion{
			Class:    bandClass(b.Label),
			Label:    b.Label,
			Range:    rng,
			Count:    c,
			PctStr:   strconv.Itoa(int(math.Round(pct))),
			WidthPct: width,
			Hint:     b.Hint,
		})
		if b.Min > 0 {
			pos := fpct(100 * b.Min / axisMax)
			ticks = append(ticks, tick{pos, strconv.FormatFloat(b.Min, 'g', -1, 64)})
			seps = append(seps, pos)
		}
	}
	data := struct {
		Rows       []row
		Regions    []bandRegion
		Bins       []bin
		Ticks      []tick
		Seps       []string
		HasDist    bool
		MedianPct  string
		MedianStr  string
		P90Str     string
		Inert      int
		MaxStr     string
		Files      int
		TotalLines int
		LowConf    int
		Logo       template.URL
		Version    string
		Date       string
	}{
		Rows:       rows,
		Regions:    regions,
		Bins:       bins,
		Ticks:      ticks,
		Seps:       seps,
		HasDist:    total > 0,
		MedianPct:  fpct(100 * med / axisMax),
		MedianStr:  fscore(med),
		P90Str:     fscore(p90),
		Inert:      inert,
		MaxStr:     fscore(maxScore),
		Files:      len(files),
		TotalLines: totalLines,
		LowConf:    lowConf,
		Logo:       logoDataURI,
		Version:    version,
		Date:       time.Now().Format("2006-01-02 15:04"),
	}

	var b strings.Builder
	if err := htmlTemplate.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{"dict": dict}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>promptcc report</title>
<script src="https://cdn.tailwindcss.com"></script>
<script>tailwind.config = { darkMode: 'media' }</script>
<style>
  :root {
    color-scheme: light;
    --page: #f4f4f2; --card: #fcfcfb; --ink: #0b0b0b; --ink2: #52514e;
    --muted: #898781; --grid: #e1e0d9; --hair: rgba(11,11,11,.10);
    --good: #0ca30c; --warn: #fab219; --serious: #ec835a; --crit: #d03b3b;
    --goodtext: #006300;
    --c1: #2a78d6; --c2: #eb6834; --c3: #1baf7a; --c4: #eda100; --c5: #e87ba4;
    --code-bg: #f6f6f4; --code-bar: #ecece9; --code-ink: #24292f; --code-num: #b3b1a9;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      color-scheme: dark;
      --page: #0d0d0d; --card: #1a1a19; --ink: #ffffff; --ink2: #c3c2b7;
      --muted: #898781; --grid: #2c2c2a; --hair: rgba(255,255,255,.12);
      --goodtext: #0ca30c;
      --c1: #3987e5; --c2: #d95926; --c3: #199e70; --c4: #c98500; --c5: #d55181;
      --code-bg: #14181d; --code-bar: #1b2027; --code-ink: #d6dae0; --code-num: #5b6673;
    }
  }
  body { background: var(--page); color: var(--ink); }
  ::selection { background: rgba(42,120,214,.25); }

  /* Wordmark: "cc" picks up the logo's blue→violet gradient */
  .brand-cc {
    background: linear-gradient(90deg, #2a78d6, #6d3fd4);
    -webkit-background-clip: text; background-clip: text; color: transparent;
  }
  @media (prefers-color-scheme: dark) {
    .brand-cc { background: linear-gradient(90deg, #3987e5, #9085e9); -webkit-background-clip: text; background-clip: text; }
  }

  /* Sidebar nav */
  .navlink { color: var(--ink2); }
  .navlink:hover { background: var(--page); color: var(--ink); }
  .navlink.active { background: rgba(42,120,214,.12); color: var(--c1); font-weight: 600; }

  /* Severity dots and fills */
  .dot.low, .fill-low { background: var(--good); }
  .dot.moderate, .fill-moderate { background: var(--warn); }
  .dot.high, .fill-high { background: var(--serious); }
  .dot.critical, .fill-critical { background: var(--crit); }
  .txt-low { color: var(--goodtext); } .txt-moderate { color: var(--warn); }
  .txt-high { color: var(--serious); } .txt-critical { color: var(--crit); }
  .edge-low { border-color: var(--good); } .edge-moderate { border-color: var(--warn); }
  .edge-high { border-color: var(--serious); } .edge-critical { border-color: var(--crit); }

  /* Histogram */
  .bin { position: absolute; top: 0; bottom: 0; padding: 0 1.5px; display: flex; align-items: flex-end; }
  .bin > i { display: block; width: 100%; border-radius: 3px 3px 0 0; }
  .sep { position: absolute; top: 0; bottom: 0; border-left: 1px dashed var(--grid); }
  .marker { position: absolute; top: 0; bottom: 0; border-left: 2px solid var(--ink2); }

  /* Code block */
  .code { background: var(--code-bg); color: var(--code-ink); }
  .code .bar { background: var(--code-bar); }
  .code .rows { display: grid; grid-template-columns: auto 1fr; }
  .code .ln {
    text-align: right; padding: 0 .9rem 0 1rem; color: var(--code-num);
    user-select: none; font-variant-numeric: tabular-nums;
  }
  .code .lc { white-space: pre-wrap; word-break: break-word; padding-right: 1rem; }
  .code .lc:empty::after { content: "\200b"; }

  /* Low-confidence toggle (pure CSS) */
  #showlow:not(:checked) ~ details.lowconf { display: none; }
</style>
</head>
<body class="font-sans text-[15px] leading-relaxed antialiased">
<nav class="fixed top-0 left-0 bottom-0 w-56 p-4 border-r flex flex-col gap-1 bg-[var(--card)] border-[var(--hair)] max-lg:static max-lg:w-auto max-lg:flex-row max-lg:items-center max-lg:flex-wrap max-lg:border-r-0 max-lg:border-b">
  <div class="flex items-center gap-2.5 px-2 mb-5 max-lg:mb-0 max-lg:mr-4">
    <img src="{{.Logo}}" alt="" class="h-8 w-auto shrink-0">
    <div>
      <div class="text-lg font-bold tracking-tight leading-none">prompt<span class="brand-cc">cc</span></div>
      {{- if ne .Version "dev"}}<div class="text-xs text-[var(--muted)] mt-0.5">{{.Version}}</div>{{end}}
    </div>
  </div>
  <a id="nav-dashboard" href="#dashboard" class="navlink px-3 py-2 rounded-md text-sm no-underline transition-colors">Dashboard</a>
  <a id="nav-explorer" href="#explorer" class="navlink px-3 py-2 rounded-md text-sm no-underline transition-colors">Explorer</a>
  <a id="nav-help" href="#help" class="navlink px-3 py-2 rounded-md text-sm no-underline transition-colors">Help</a>
  <div class="mt-auto max-lg:mt-0 max-lg:ml-auto max-lg:flex max-lg:items-center max-lg:gap-3">
    <a href="https://github.com/halleck45/promptcc" class="block px-3 py-2 text-sm text-[var(--muted)] no-underline hover:text-[var(--ink)]">GitHub ↗</a>
    <div class="px-3 max-lg:px-0 text-xs text-[var(--muted)]">generated {{.Date}}</div>
  </div>
</nav>

<main class="ml-56 max-lg:ml-0 px-6 py-8 max-w-6xl">

  <!-- ============ DASHBOARD ============ -->
  <section id="page-dashboard">
    <header class="mb-6">
      <h1 class="text-2xl font-bold tracking-tight">Dashboard</h1>
      <p class="text-sm text-[var(--ink2)] mt-2 max-w-2xl">The <b>branching score</b> counts the decision logic a prompt encodes, not its length.
        Higher means more conditional behavior to keep consistent. <a href="#help" class="text-[var(--c1)] no-underline hover:underline">How it works →</a></p>
    </header>

    {{- if .HasDist}}
    <div class="grid grid-cols-2 sm:grid-cols-4 xl:grid-cols-7 gap-3 mb-6">
      {{template "kpi" dict "V" (len .Rows) "K" "prompts"}}
      {{template "kpi" dict "V" .TotalLines "K" "prompt lines"}}
      {{template "kpi" dict "V" .Files "K" "files"}}
      {{template "kpi" dict "V" .MedianStr "K" "median score (scored > 0)"}}
      {{template "kpi" dict "V" .P90Str "K" "p90 (scored > 0)"}}
      {{template "kpi" dict "V" .Inert "K" "inert fragments (score 0)"}}
      {{template "kpi" dict "V" .MaxStr "K" "max"}}
    </div>

    <div class="rounded-xl border p-5 mb-5 bg-[var(--card)] border-[var(--hair)]">
      <h2 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mb-3">Score distribution</h2>
      <div class="relative" style="height:130px">
        <div class="absolute inset-0 border-b border-[var(--grid)]">
          {{- range .Bins}}
          <div class="bin" style="left:{{.LeftPct}}%;width:{{.WidthPct}}%" title="{{.Title}}"><i class="fill-{{.Class}}" style="height:{{.HeightPct}}%"></i></div>
          {{- end}}
          {{- range .Seps}}<div class="sep" style="left:{{.}}%"></div>{{end}}
          <div class="marker" style="left:{{.MedianPct}}%"><span class="absolute -top-4 -left-1 whitespace-nowrap text-xs text-[var(--ink2)]">median {{.MedianStr}}</span></div>
        </div>
      </div>
      <div class="relative h-5 mt-1 text-xs text-[var(--muted)]">
        {{- range .Ticks}}<span class="absolute -translate-x-1/2 tabular-nums" style="left:{{.LeftPct}}%">{{.Label}}</span>{{end}}
      </div>
    </div>

    <div class="rounded-xl border p-5 bg-[var(--card)] border-[var(--hair)]">
      <h2 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mb-3">Severity breakdown</h2>
      <div class="flex h-3 rounded-full overflow-hidden gap-0.5 mb-4">
        {{- range .Regions}}{{if .Count}}<div class="fill-{{.Class}}" style="width:{{.WidthPct}}%" title="{{.Label}}: {{.Count}} ({{.PctStr}}%)"></div>{{end}}{{end}}
      </div>
      <div class="grid sm:grid-cols-2 gap-x-8 gap-y-1.5">
        {{- range .Regions}}
        <div class="flex items-baseline gap-2 text-sm">
          <span class="dot {{.Class}} w-2.5 h-2.5 rounded-sm shrink-0 translate-y-0.5"></span>
          <span class="font-semibold text-xs tracking-wide w-20">{{.Label}}</span>
          <span class="tabular-nums w-10 text-right">{{.Count}}</span>
          <span class="text-[var(--muted)] tabular-nums w-10 text-right">{{.PctStr}}%</span>
          <span class="text-[var(--muted)] truncate">{{.Hint}}</span>
        </div>
        {{- end}}
      </div>
    </div>
    {{- else}}
    <p class="text-[var(--muted)]">No prompts found.</p>
    {{- end}}
  </section>

  <!-- ============ EXPLORER ============ -->
  <section id="page-explorer" class="hidden">
    <header class="mb-5">
      <h1 class="text-2xl font-bold tracking-tight">Explorer</h1>
      <p class="text-[var(--muted)] mt-1">Sorted by branching score, highest first. Click a prompt to expand it.</p>
    </header>

    {{- if .LowConf}}
    <input type="checkbox" id="showlow" class="align-middle w-4 h-4 mr-2 accent-[var(--c1)] cursor-pointer">
    <label for="showlow" class="inline-block mb-4 text-sm text-[var(--ink2)] cursor-pointer select-none">Show {{.LowConf}} low-confidence prompt(s)</label>
    {{- end}}

    {{- range .Rows}}
    <details class="group mb-2 rounded-xl border bg-[var(--card)] border-[var(--hair)] border-l-[3px] edge-{{.BandClass}} overflow-hidden{{if .LowConf}} lowconf{{end}}">
      <summary class="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-3 cursor-pointer list-none [&::-webkit-details-marker]:hidden">
        <span class="font-bold tabular-nums w-14 text-lg">{{.ScoreStr}}</span>
        <span class="inline-flex items-center gap-1.5 text-xs font-bold tracking-wide txt-{{.BandClass}}"><span class="dot {{.BandClass}} w-2 h-2 rounded-sm"></span>{{.Band.Label}}</span>
        <span class="font-mono text-sm" title="{{.File}}:{{.Line}}"><span class="text-[var(--muted)]">{{.Dir}}</span><b>{{.Base}}</b>:{{.Line}}</span>
        {{- if .Ctx}}<span class="text-sm text-[var(--muted)] truncate">{{.Ctx}}</span>{{end}}
        <span class="ml-auto text-xs text-[var(--muted)] border rounded-full px-2 py-0.5 border-[var(--hair)]">confidence {{.Confidence}}</span>
        <span class="text-[var(--muted)] transition-transform group-open:rotate-90">›</span>
      </summary>
      <div class="px-4 pb-4 border-t border-[var(--hair)]">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mt-3 mb-2">Signals</h3>
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
          {{- range .Cards}}
          <div class="rounded-lg p-2.5 {{if .Control}}border border-dashed border-[var(--grid)]{{else}}bg-[var(--page)]{{end}}{{if .Dim}} opacity-45{{end}}">
            <div class="text-lg font-bold leading-none">{{.Value}}</div>
            <div class="text-xs text-[var(--muted)] mt-1">{{.Label}}</div>
            {{- if .Points}}<div class="text-xs tabular-nums mt-0.5 {{if .Relief}}txt-low{{else}}text-[var(--ink2)]{{end}}">{{.Points}}</div>{{end}}
          </div>
          {{- end}}
        </div>

        {{- if or .Channels .Slots}}
        <div class="flex flex-wrap gap-x-10 gap-y-3 mt-4">
          {{- if .Channels}}
          <div>
            <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mb-1.5">Injection channels</h3>
            <div class="flex flex-wrap gap-1.5">
              {{- range .Channels}}<span class="text-xs px-2 py-0.5 rounded-full border bg-[var(--page)] border-[var(--hair)]"><b class="tabular-nums">{{.N}}×</b> {{.K}}</span>{{end}}
            </div>
          </div>
          {{- end}}
          {{- if .Slots}}
          <div>
            <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mb-1.5">Interpolated values</h3>
            <div class="flex flex-wrap gap-1.5">
              {{- range .Slots}}<code class="text-xs px-1.5 py-0.5 rounded border font-mono bg-[var(--page)] border-[var(--hair)]">{{.}}</code>{{end}}
            </div>
          </div>
          {{- end}}
        </div>
        {{- end}}

        <h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--muted)] mt-4 mb-2">Prompt text</h3>
        <div class="code rounded-lg border border-[var(--hair)] overflow-hidden text-[13px] font-mono">
          <div class="bar flex items-center gap-1.5 px-3 py-2 border-b border-[var(--hair)]">
            <span class="w-2.5 h-2.5 rounded-full bg-[var(--crit)] opacity-70"></span>
            <span class="w-2.5 h-2.5 rounded-full bg-[var(--warn)] opacity-70"></span>
            <span class="w-2.5 h-2.5 rounded-full bg-[var(--good)] opacity-70"></span>
            <span class="ml-2 text-xs text-[var(--code-num)]">{{.Base}}:{{.Line}}</span>
          </div>
          <div class="rows py-2 max-h-[26rem] overflow-auto">
            {{- range .Code}}<span class="ln">{{.No}}</span><span class="lc">{{.Text}}</span>{{end}}
          </div>
        </div>
      </div>
    </details>
    {{- end}}
  </section>

  <!-- ============ HELP ============ -->
  <section id="page-help" class="hidden">
    <header class="mb-5">
      <h1 class="text-2xl font-bold tracking-tight">Help</h1>
      <p class="text-[var(--muted)] mt-1">How the branching score works and how to read the report.</p>
    </header>

    <div class="rounded-xl border p-5 mb-4 bg-[var(--card)] border-[var(--hair)] max-w-3xl">
      <h2 class="font-semibold mb-2">The idea</h2>
      <p class="text-[var(--ink2)] text-sm">A prompt's maintenance cost tracks the <b>decision logic</b> it encodes, not how long it is.
      promptcc counts the branching signals below and sums them into one score. Prompt length is reported only as a control.
      Weights are a documented v0 heuristic, not calibrated coefficients.</p>
    </div>

    <div class="rounded-xl border p-5 mb-4 bg-[var(--card)] border-[var(--hair)] max-w-3xl">
      <h2 class="font-semibold mb-1">The signals</h2>
      <p class="text-sm text-[var(--muted)] mb-3">score = sum of the weighted signals, minus guardrail relief.</p>
      <table class="w-full text-sm border-collapse">
        <thead><tr class="text-left text-[var(--muted)] text-xs uppercase tracking-wide">
          <th class="py-1.5 pr-4 font-semibold">Signal</th><th class="py-1.5 pr-4 font-semibold">What it counts</th><th class="py-1.5 font-semibold whitespace-nowrap">Weight</th>
        </tr></thead>
        <tbody>
          {{template "sig" dict "S" "decision points" "D" "conditional behaviors (if, unless, quand, …)" "W" "×1.0"}}
          {{template "sig" dict "S" "decision density" "D" "share of instructions that are conditional" "W" "×10"}}
          {{template "sig" dict "S" "routing / escalation" "D" "tool, agent or channel routing points" "W" "×1.5"}}
          {{template "sig" dict "S" "injection channels" "D" "distinct mechanisms injecting runtime values" "W" "×1.0"}}
          {{template "sig" dict "S" "output schema depth" "D" "nesting depth of the required output structure" "W" "×0.5"}}
          {{template "sig" dict "S" "explicit guardrails" "D" "hard rules (never, always, …); these make a prompt easier to maintain" "W" "−0.3 ea."}}
        </tbody>
      </table>
      <p class="text-xs text-[var(--muted)] mt-2">Guardrail relief is capped at 40% of the summed signals; the score never goes below 0.</p>
    </div>

    <div class="rounded-xl border p-5 mb-4 bg-[var(--card)] border-[var(--hair)] max-w-3xl">
      <h2 class="font-semibold mb-3">Severity bands</h2>
      <div class="space-y-2">
        {{- range .Regions}}
        <div class="flex items-baseline gap-3 text-sm">
          <span class="dot {{.Class}} w-2.5 h-2.5 rounded-sm shrink-0 translate-y-0.5"></span>
          <span class="font-semibold text-xs tracking-wide w-20">{{.Label}}</span>
          <span class="tabular-nums text-[var(--ink2)] w-36">{{.Range}}</span>
          <span class="text-[var(--muted)]">{{.Hint}}</span>
        </div>
        {{- end}}
      </div>
    </div>

    <div class="rounded-xl border p-5 bg-[var(--card)] border-[var(--hair)] max-w-3xl">
      <h2 class="font-semibold mb-2">Reading a prompt</h2>
      <ul class="text-sm text-[var(--ink2)] space-y-2 list-disc pl-5">
        <li>The <b>Signals</b> cards decompose the score: each shows the metric and its point contribution, so you can see <i>why</i> a prompt is flagged. Zeroed signals are dimmed.</li>
        <li><b>Confidence</b> is how sure the extractor is that the string is an LLM prompt:
          <span class="txt-low font-medium">high</span> = seen at an LLM API call site,
          <b>medium</b> = bound to a prompt-like name,
          <b>low</b> = matched by text heuristics only. Low-confidence prompts are hidden by default in the Explorer but still counted here.</li>
      </ul>
    </div>

    <footer class="text-xs text-[var(--muted)] mt-6">promptcc {{.Version}} · branching, not volume</footer>
  </section>
</main>

<script>
  const pages = ['dashboard', 'explorer', 'help'];
  function show(p) {
    if (!pages.includes(p)) p = 'dashboard';
    for (const x of pages) {
      document.getElementById('page-' + x).classList.toggle('hidden', x !== p);
      document.getElementById('nav-' + x).classList.toggle('active', x === p);
    }
    window.scrollTo(0, 0);
  }
  addEventListener('hashchange', () => show(location.hash.slice(1)));
  show(location.hash.slice(1) || 'dashboard');
</script>
</body>
</html>
`))

func init() {
	template.Must(htmlTemplate.Parse(`{{define "kpi"}}<div class="rounded-xl border p-4 bg-[var(--card)] border-[var(--hair)]"><div class="text-2xl font-semibold">{{.V}}</div><div class="text-xs text-[var(--muted)] mt-0.5">{{.K}}</div></div>{{end}}`))
	template.Must(htmlTemplate.Parse(`{{define "sig"}}<tr class="border-t border-[var(--hair)]"><td class="py-2 pr-4 align-top">{{.S}}</td><td class="py-2 pr-4 align-top text-[var(--ink2)]">{{.D}}</td><td class="py-2 align-top tabular-nums whitespace-nowrap">{{.W}}</td></tr>{{end}}`))
}

// dict builds a map from alternating key/value pairs, for passing multiple
// values into a template block.
func dict(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[fmt.Sprint(pairs[i])] = pairs[i+1]
	}
	return m
}
