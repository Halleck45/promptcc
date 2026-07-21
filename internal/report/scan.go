package report

import (
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

// distBins is the number of histogram bins in the score distribution.
const distBins = 48

type chip struct {
	K     string
	N     int
	Alpha string // background wash opacity, scaled to relative frequency
}

type segment struct {
	Label    string
	Points   string // formatted contribution, e.g. "+8.0"
	WidthPct string
	Color    int // categorical slot 1..5, fixed per component
}

type card struct {
	Value   string
	Label   string
	Points  string // contribution to the score, e.g. "+8.0 pts"
	Dim     bool   // zero value: de-emphasized
	Relief  bool   // guardrails: subtracts from the score
	Control bool   // volume metrics: control group
}

type bin struct {
	HeightPct string
	Title     string
	Empty     bool
}

type bandRegion struct {
	Class    string
	LeftPct  string
	WidthPct string
	Label    string
	Range    string
	Count    int
	Hint     string
}

type tick struct {
	LeftPct string
	Label   string
}

// bandClass maps a band label to a CSS class name.
func bandClass(label string) string { return strings.ToLower(label) }

func fpct(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

func fscore(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func fpts(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', 1, 64) }

// shortDir keeps at most the last two directories of a path, for display.
// The full path stays available in the element's title attribute.
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

// highlightDecisions escapes text and wraps the decision keyword occurrences
// counted by the analyzer in <mark> tags, linking metric to source.
func highlightDecisions(text string) template.HTML {
	spans := analyzer.DecisionSpans(text)
	var b strings.Builder
	last := 0
	for _, s := range spans {
		b.WriteString(template.HTMLEscapeString(text[last:s[0]]))
		b.WriteString("<mark>")
		b.WriteString(template.HTMLEscapeString(text[s[0]:s[1]]))
		b.WriteString("</mark>")
		last = s[1]
	}
	b.WriteString(template.HTMLEscapeString(text[last:]))
	return template.HTML(b.String())
}

func keywordChips(m map[string]int) []chip {
	kvs := sortedByCount(m)
	maxN := 0
	for _, e := range kvs {
		if e.N > maxN {
			maxN = e.N
		}
	}
	out := make([]chip, 0, len(kvs))
	for _, e := range kvs {
		alpha := 0.14
		if maxN > 0 {
			alpha += 0.36 * float64(e.N) / float64(maxN)
		}
		out = append(out, chip{e.K, e.N, strconv.FormatFloat(alpha, 'f', 2, 64)})
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
	maxRaw := 0.0
	for _, e := range sorted {
		scores = append(scores, e.Metrics.BranchingScore)
		files[e.File] = true
		if _, _, raw := analyzer.Breakdown(&e.Metrics); raw > maxRaw {
			maxRaw = raw
		}
	}
	asc := make([]float64, len(scores))
	copy(asc, scores)
	sort.Float64s(asc)

	med := median(asc)
	p90 := quantile(asc, 0.9)
	maxScore := 0.0
	if n := len(asc); n > 0 {
		maxScore = asc[n-1]
	}
	scale := analyzer.BandScale()
	axisMax := math.Max(maxScore*1.05, scale[len(scale)-1].Min+2)

	type row struct {
		ScanEntry
		Band        analyzer.Band
		BandClass   string
		LowConf     bool
		Dir, Base   string
		Percentile  string
		VsMedian    string
		ScoreStr    string
		RawStr      string
		ReliefStr   string
		BarPct      string
		FinalPct    string
		HasRelief   bool
		Segments    []segment
		Cards       []card
		Keywords    []chip
		Channels    []kv
		Highlighted template.HTML
	}

	bandCounts := map[string]int{}
	rows := make([]row, 0, len(sorted))
	for _, e := range sorted {
		m := e.Metrics
		band := analyzer.BandFor(m.BranchingScore)
		bandCounts[band.Label]++
		dir, base := path.Split(e.File)

		countLE := 0
		for _, s := range asc {
			if s <= m.BranchingScore {
				countLE++
			}
		}
		percentile := ""
		if n := len(asc); n > 1 {
			percentile = fmt.Sprintf("p%d", int(math.Round(100*float64(countLE)/float64(n))))
		}
		vsMedian := ""
		if med > 0 && len(asc) > 1 {
			vsMedian = fmt.Sprintf("×%.1f median", m.BranchingScore/med)
		}

		parts, relief, raw := analyzer.Breakdown(&m)
		var segs []segment
		for i, p := range parts {
			if p.Value <= 0 {
				continue
			}
			segs = append(segs, segment{
				Label:    p.Label,
				Points:   "+" + fpts(p.Value),
				WidthPct: fpct(100 * p.Value / raw),
				Color:    i + 1,
			})
		}
		barPct := "0"
		if maxRaw > 0 {
			barPct = fpct(100 * raw / maxRaw)
		}
		finalPct := "100"
		if raw > 0 {
			finalPct = fpct(100 * m.BranchingScore / raw)
		}

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
			ScanEntry:   e,
			Band:        band,
			BandClass:   bandClass(band.Label),
			LowConf:     e.Confidence == "low",
			Dir:         shortDir(dir),
			Base:        base,
			Percentile:  percentile,
			VsMedian:    vsMedian,
			ScoreStr:    fscore(m.BranchingScore),
			RawStr:      fscore(raw),
			ReliefStr:   fpts(relief),
			BarPct:      barPct,
			FinalPct:    finalPct,
			HasRelief:   relief > 0,
			Segments:    segs,
			Cards:       cards,
			Keywords:    keywordChips(m.Detail.DecisionsByKeyword),
			Channels:    sortedByCount(m.Detail.InjectionByChannel),
			Highlighted: highlightDecisions(e.Text),
		})
	}

	// Histogram bins over [0, axisMax].
	counts := make([]int, distBins)
	maxCount := 0
	for _, s := range scores {
		i := int(float64(distBins) * s / axisMax)
		if i >= distBins {
			i = distBins - 1
		}
		counts[i]++
		if counts[i] > maxCount {
			maxCount = counts[i]
		}
	}
	bins := make([]bin, distBins)
	binW := axisMax / distBins
	for i, c := range counts {
		if c == 0 {
			bins[i] = bin{HeightPct: "0", Empty: true}
			continue
		}
		h := math.Max(4, 100*float64(c)/float64(maxCount))
		bins[i] = bin{
			HeightPct: fpct(h),
			Title: fmt.Sprintf("score %.1f to %.1f: %d prompt(s)",
				float64(i)*binW, float64(i+1)*binW, c),
		}
	}

	// Severity band regions (histogram washes and legend rows).
	regions := make([]bandRegion, 0, len(scale))
	var ticks []tick
	ticks = append(ticks, tick{"0", "0"})
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
		regions = append(regions, bandRegion{
			Class:    bandClass(b.Label),
			LeftPct:  fpct(100 * b.Min / axisMax),
			WidthPct: fpct(100 * (next - b.Min) / axisMax),
			Label:    b.Label,
			Range:    rng,
			Count:    bandCounts[b.Label],
			Hint:     b.Hint,
		})
		if b.Min > 0 {
			ticks = append(ticks, tick{fpct(100 * b.Min / axisMax), strconv.FormatFloat(b.Min, 'g', -1, 64)})
		}
	}

	data := struct {
		Rows      []row
		Regions   []bandRegion
		Bins      []bin
		Ticks     []tick
		HasDist   bool
		MedianPct string
		MedianStr string
		P90Str    string
		MaxStr    string
		Files     int
		Version   string
		Date      string
	}{
		Rows:      rows,
		Regions:   regions,
		Bins:      bins,
		Ticks:     ticks,
		HasDist:   len(rows) > 0,
		MedianPct: fpct(100 * med / axisMax),
		MedianStr: fscore(med),
		P90Str:    fscore(p90),
		MaxStr:    fscore(maxScore),
		Files:     len(files),
		Version:   version,
		Date:      time.Now().Format("2006-01-02 15:04"),
	}

	var b strings.Builder
	if err := htmlTemplate.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

var htmlTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>promptcc report</title>
<style>
  :root {
    color-scheme: light;
    --page: #f9f9f7; --card: #fcfcfb; --ink: #0b0b0b; --ink2: #52514e;
    --muted: #898781; --grid: #e1e0d9; --hair: rgba(11,11,11,.10);
    --good: #0ca30c; --warn: #fab219; --serious: #ec835a; --crit: #d03b3b;
    --goodtext: #006300;
    --c1: #2a78d6; --c2: #eb6834; --c3: #1baf7a; --c4: #eda100; --c5: #e87ba4;
    --kw: 42 120 214;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      color-scheme: dark;
      --page: #0d0d0d; --card: #1a1a19; --ink: #ffffff; --ink2: #c3c2b7;
      --muted: #898781; --grid: #2c2c2a; --hair: rgba(255,255,255,.10);
      --goodtext: #0ca30c;
      --c1: #3987e5; --c2: #d95926; --c3: #199e70; --c4: #c98500; --c5: #d55181;
      --kw: 57 135 229;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 2rem 1rem; background: var(--page); color: var(--ink);
    font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
  }
  main { max-width: 62rem; margin: 0 auto; }
  h1 { font-size: 1.4rem; margin: 0; }
  h1 span { color: var(--muted); font-weight: normal; }
  .meta { color: var(--muted); margin: .25rem 0 .75rem; }
  .explain {
    color: var(--ink2); font-size: .88rem; max-width: 52rem; margin: 0 0 1.25rem;
  }
  .explain b { color: var(--ink); }
  h3 {
    font-size: .78rem; text-transform: uppercase; letter-spacing: .05em;
    color: var(--muted); margin: 1.1rem 0 .4rem; font-weight: 600;
  }
  .panel {
    background: var(--card); border: 1px solid var(--hair); border-radius: 10px;
    padding: 1rem 1.25rem 1.25rem; margin-bottom: 1.5rem;
  }
  .tiles { display: flex; flex-wrap: wrap; gap: 2rem; margin-bottom: 1rem; }
  .tile .v { font-size: 1.5rem; font-weight: 600; }
  .tile .k { color: var(--muted); font-size: .78rem; }

  /* Score distribution */
  .plot { position: relative; height: 88px; margin-top: .5rem; }
  .wash { position: absolute; top: 0; bottom: 0; opacity: .09; }
  .wash.low { background: var(--good); } .wash.moderate { background: var(--warn); }
  .wash.high { background: var(--serious); } .wash.critical { background: var(--crit); }
  .bins {
    position: absolute; inset: 0; display: flex; align-items: flex-end; gap: 1px;
    border-bottom: 1px solid var(--grid);
  }
  .bin { flex: 1; height: 100%; display: flex; align-items: flex-end; }
  .bin i { display: block; width: 100%; background: var(--c1); border-radius: 2px 2px 0 0; }
  .marker { position: absolute; top: 0; bottom: 0; border-left: 1px solid var(--ink2); }
  .marker span {
    position: absolute; top: -1.15rem; left: -.2rem; white-space: nowrap;
    font-size: .72rem; color: var(--ink2);
  }
  .axis { position: relative; height: 1.2rem; color: var(--muted); font-size: .72rem; }
  .axis span { position: absolute; transform: translateX(-50%); font-variant-numeric: tabular-nums; }
  .bands { list-style: none; margin: 1rem 0 0; padding: 0; font-size: .85rem; }
  .bands li {
    display: grid; grid-template-columns: 14px 6.5rem 9.5rem 3.5rem 1fr;
    gap: .6rem; align-items: baseline; padding: .15rem 0;
  }
  .bands i { width: 10px; height: 10px; border-radius: 3px; display: inline-block; position: relative; top: 1px; }
  .bands .low i { background: var(--good); } .bands .moderate i { background: var(--warn); }
  .bands .high i { background: var(--serious); } .bands .critical i { background: var(--crit); }
  .bands b { font-size: .78rem; letter-spacing: .04em; }
  .bands .rng { color: var(--ink2); font-variant-numeric: tabular-nums; }
  .bands .cnt { font-variant-numeric: tabular-nums; text-align: right; }
  .bands .hint { color: var(--muted); }

  /* Prompt cards */
  details.prompt {
    background: var(--card); border: 1px solid var(--hair);
    border-left: 4px solid var(--grid); border-radius: 8px;
    margin-bottom: .6rem; overflow: hidden;
  }
  details.prompt.critical { border-left-color: var(--crit); }
  details.prompt.high     { border-left-color: var(--serious); }
  details.prompt.moderate { border-left-color: var(--warn); }
  details.prompt.low      { border-left-color: var(--good); }
  details.prompt.lowconf  { border-left-style: dashed; }
  summary {
    display: flex; flex-wrap: wrap; gap: .35rem .9rem; align-items: baseline;
    padding: .7rem 1rem; cursor: pointer; list-style: none;
  }
  summary::-webkit-details-marker { display: none; }
  .score { font-weight: 700; font-variant-numeric: tabular-nums; min-width: 3.6rem; }
  .badge { font-size: .75rem; font-weight: 700; letter-spacing: .04em; color: var(--ink2); }
  .badge i { width: 9px; height: 9px; border-radius: 3px; display: inline-block; margin-right: .3rem; }
  .badge.low i { background: var(--good); } .badge.moderate i { background: var(--warn); }
  .badge.high i { background: var(--serious); } .badge.critical i { background: var(--crit); }
  .pos { color: var(--muted); font-size: .8rem; font-variant-numeric: tabular-nums; }
  .loc { font-family: ui-monospace, monospace; font-size: .85rem; }
  .loc .dir { color: var(--muted); }
  .ctx { color: var(--muted); font-size: .85rem; }
  .conf {
    margin-left: auto; font-size: .72rem; color: var(--muted);
    border: 1px solid var(--hair); border-radius: 99px; padding: .05rem .55rem;
  }
  .conf.low { border-style: dashed; border-color: var(--muted); }
  .body { padding: 0 1rem 1rem; border-top: 1px solid var(--hair); }

  /* Score composition */
  .bar {
    position: relative; display: flex; gap: 2px; height: 14px;
    min-width: 3rem; margin: .2rem 0 .45rem;
  }
  .bar i { display: block; height: 100%; }
  .bar i:last-of-type { border-radius: 0 4px 4px 0; }
  .c1 { background: var(--c1); } .c2 { background: var(--c2); } .c3 { background: var(--c3); }
  .c4 { background: var(--c4); } .c5 { background: var(--c5); }
  .bar .hatch {
    position: absolute; top: 0; bottom: 0; right: 0;
    background: repeating-linear-gradient(135deg, var(--card) 0 3px, transparent 3px 6px);
  }
  .bar .mark { position: absolute; top: -2px; bottom: -2px; border-left: 2px solid var(--ink); }
  .comp .legend { margin: 0; font-size: .82rem; color: var(--ink2); }
  .comp .legend span { margin-right: 1rem; white-space: nowrap; }
  .comp .legend .sw {
    width: 9px; height: 9px; border-radius: 3px; display: inline-block; margin-right: .3rem;
  }
  .comp .legend .relief { color: var(--goodtext); }
  .comp .legend .tot { font-weight: 700; color: var(--ink); }

  /* Metric cards */
  .grid {
    display: grid; grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
    gap: .5rem; margin: .4rem 0 0;
  }
  .metric { background: var(--page); border-radius: 6px; padding: .5rem .7rem; }
  .metric .v { font-size: 1.15rem; font-weight: 700; }
  .metric .k { color: var(--muted); font-size: .78rem; }
  .metric .pts { font-size: .78rem; color: var(--ink2); font-variant-numeric: tabular-nums; }
  .metric .pts.relief { color: var(--goodtext); }
  .metric.dim { opacity: .45; }
  .metric.control { border: 1px dashed var(--grid); background: transparent; }

  /* Evidence chips */
  .lists { display: flex; flex-wrap: wrap; gap: .25rem 2.5rem; }
  .chips { display: flex; flex-wrap: wrap; gap: .35rem; }
  .kwchip {
    font-size: .82rem; padding: .1rem .5rem; border-radius: 99px;
    background: rgb(var(--kw) / var(--a)); border: 1px solid var(--hair);
  }
  .kwchip b { font-variant-numeric: tabular-nums; font-weight: 600; }
  .chip {
    font-size: .82rem; padding: .1rem .5rem; border-radius: 99px;
    border: 1px solid var(--hair); background: var(--page);
  }
  .chip b { font-variant-numeric: tabular-nums; font-weight: 600; }
  code.slot {
    font-size: .78rem; padding: .1rem .4rem; border-radius: 5px;
    border: 1px solid var(--hair); background: var(--page);
    font-family: ui-monospace, monospace;
  }
  pre.text {
    background: var(--page); border: 1px solid var(--hair); border-radius: 6px;
    padding: .8rem; font-size: .82rem; white-space: pre-wrap; word-break: break-word;
    max-height: 26rem; overflow: auto; margin: 0;
  }
  pre.text mark {
    background: rgb(var(--kw) / .28); color: inherit;
    border-radius: 3px; padding: 0 1px;
  }
  footer { color: var(--muted); font-size: .8rem; margin-top: 2rem; }
</style>
</head>
<body>
<main>
  <h1>promptcc <span>· branching complexity report</span></h1>
  <p class="meta">{{len .Rows}} prompt(s) in {{.Files}} file(s) · generated {{.Date}} · promptcc {{.Version}}</p>
  <p class="explain">
    <b>What the score measures.</b> The branching score estimates how much decision
    logic a prompt encodes; length alone predicts nothing. It is a weighted sum of five
    signals (decision points ×1.0, decision density ×10, routing ×1.5, injection
    channels ×1.0, output schema depth ×0.5); each explicit guardrail then relieves
    0.3 points, capped at 40% of the raw sum. The scale is open ended and 0 means a
    fully linear prompt. Weights are a documented v0 heuristic, not calibrated coefficients.
  </p>

  {{- if .HasDist}}
  <div class="panel">
    <div class="tiles">
      <div class="tile"><div class="v">{{len .Rows}}</div><div class="k">prompts</div></div>
      <div class="tile"><div class="v">{{.Files}}</div><div class="k">files</div></div>
      <div class="tile"><div class="v">{{.MedianStr}}</div><div class="k">median score</div></div>
      <div class="tile"><div class="v">{{.P90Str}}</div><div class="k">p90</div></div>
      <div class="tile"><div class="v">{{.MaxStr}}</div><div class="k">max</div></div>
    </div>
    <h3>Score distribution</h3>
    <div class="plot">
      {{- range .Regions}}
      <div class="wash {{.Class}}" style="left:{{.LeftPct}}%;width:{{.WidthPct}}%"></div>
      {{- end}}
      <div class="bins">
        {{- range .Bins}}
        <div class="bin"{{if not .Empty}} title="{{.Title}}"{{end}}>{{if not .Empty}}<i style="height:{{.HeightPct}}%"></i>{{end}}</div>
        {{- end}}
      </div>
      <div class="marker" style="left:{{.MedianPct}}%"><span>median {{.MedianStr}}</span></div>
    </div>
    <div class="axis">
      {{- range .Ticks}}<span style="left:{{.LeftPct}}%">{{.Label}}</span>{{end}}
    </div>
    <ul class="bands">
      {{- range .Regions}}
      <li class="{{.Class}}"><i></i><b>{{.Label}}</b><span class="rng">{{.Range}}</span><span class="cnt">{{.Count}}</span><span class="hint">{{.Hint}}</span></li>
      {{- end}}
    </ul>
  </div>
  {{- end}}

  {{- range .Rows}}
  <details class="prompt {{.BandClass}}{{if .LowConf}} lowconf{{end}}">
    <summary>
      <span class="score">{{.ScoreStr}}</span>
      <span class="badge {{.BandClass}}"><i></i>{{.Band.Label}}</span>
      {{- if .Percentile}}<span class="pos">{{.Percentile}}{{if .VsMedian}} · {{.VsMedian}}{{end}}</span>{{end}}
      <span class="loc" title="{{.File}}:{{.Line}}"><span class="dir">{{.Dir}}</span><b>{{.Base}}</b>:{{.Line}}</span>
      <span class="ctx">{{.Context}}</span>
      <span class="conf {{.Confidence}}">confidence {{.Confidence}}</span>
    </summary>
    <div class="body">
      {{- if .Segments}}
      <div class="comp">
        <h3>Score composition</h3>
        <div class="bar" style="width:{{.BarPct}}%" title="raw {{.RawStr}} pts; width scaled to the highest raw score in this report">
          {{- range .Segments}}<i class="c{{.Color}}" style="width:{{.WidthPct}}%" title="{{.Label}} {{.Points}}"></i>{{end}}
          {{- if .HasRelief}}<span class="hatch" style="left:{{.FinalPct}}%"></span><span class="mark" style="left:{{.FinalPct}}%"></span>{{end}}
        </div>
        <p class="legend">
          {{- range .Segments}}<span><i class="sw c{{.Color}}"></i>{{.Label}} {{.Points}}</span>{{end}}
          {{- if .HasRelief}}<span class="relief">guardrail relief −{{.ReliefStr}}</span>{{end}}
          <span class="tot">= {{.ScoreStr}}</span>
        </p>
      </div>
      {{- end}}
      <h3>Signals</h3>
      <div class="grid">
        {{- range .Cards}}
        <div class="metric{{if .Dim}} dim{{end}}{{if .Control}} control{{end}}">
          <div class="v">{{.Value}}</div>
          <div class="k">{{.Label}}</div>
          {{- if .Points}}<div class="pts{{if .Relief}} relief{{end}}">{{.Points}}</div>{{end}}
        </div>
        {{- end}}
      </div>
      {{- if or .Keywords .Channels .Slots}}
      <div class="lists">
        {{- if .Keywords}}
        <div><h3>Decision keywords</h3><div class="chips">
          {{- range .Keywords}}<span class="kwchip" style="--a:{{.Alpha}}"><b>{{.N}}×</b> {{.K}}</span>{{end}}
        </div></div>
        {{- end}}
        {{- if .Channels}}
        <div><h3>Injection channels</h3><div class="chips">
          {{- range .Channels}}<span class="chip"><b>{{.N}}×</b> {{.K}}</span>{{end}}
        </div></div>
        {{- end}}
        {{- if .Slots}}
        <div><h3>Interpolated values</h3><div class="chips">
          {{- range .Slots}}<code class="slot">{{.}}</code>{{end}}
        </div></div>
        {{- end}}
      </div>
      {{- end}}
      <h3>Prompt text <span style="text-transform:none;letter-spacing:0">(decision keywords highlighted)</span></h3>
      <pre class="text">{{.Highlighted}}</pre>
    </div>
  </details>
  {{- end}}

  <footer>promptcc {{.Version}} · branching, not volume</footer>
</main>
</body>
</html>
`))
