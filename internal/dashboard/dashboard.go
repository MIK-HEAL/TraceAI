package dashboard

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

type Dashboard struct {
	engine *analytics.Engine
}

type behaviorProfile struct {
	Label string
	Read  int64
	Write int64
	Other int64
	Total int64
}

type pageData struct {
	Title string
	Body  template.HTML
}

const dashboardTokenCookie = "traceai_dashboard_token"

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0b1020;
      --panel: rgba(15, 23, 42, 0.82);
      --panel-border: rgba(148, 163, 184, 0.18);
      --text: #e5e7eb;
      --muted: #94a3b8;
      --accent: #38bdf8;
      --good: #22c55e;
      --warn: #f59e0b;
      --bad: #fb7185;
    }
    body {
      margin: 0;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background:
        radial-gradient(circle at top left, rgba(56, 189, 248, 0.18), transparent 30%),
        radial-gradient(circle at top right, rgba(34, 197, 94, 0.12), transparent 24%),
        linear-gradient(180deg, #050816 0%, var(--bg) 100%);
      color: var(--text);
    }
    .wrap {
      max-width: 1200px;
      margin: 0 auto;
      padding: 32px 20px 48px;
    }
    .hero {
      display: flex;
      justify-content: space-between;
      gap: 20px;
      align-items: end;
      margin-bottom: 24px;
    }
    .hero h1 {
      margin: 0;
      font-size: clamp(2rem, 5vw, 3.25rem);
      letter-spacing: -0.04em;
    }
    .hero p {
      margin: 8px 0 0;
      color: var(--muted);
      max-width: 60ch;
      line-height: 1.5;
    }
    .nav {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
    }
    .nav a {
      color: var(--text);
      text-decoration: none;
      padding: 8px 12px;
      border-radius: 999px;
      border: 1px solid var(--panel-border);
      background: rgba(15, 23, 42, 0.35);
    }
    .grid {
      display: grid;
      gap: 16px;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      margin: 18px 0 24px;
    }
    .card, .panel {
      border: 1px solid var(--panel-border);
      background: var(--panel);
      border-radius: 18px;
      box-shadow: 0 18px 55px rgba(2, 6, 23, 0.35);
      backdrop-filter: blur(12px);
    }
    .card {
      padding: 16px;
    }
    .card .label {
      color: var(--muted);
      font-size: 0.85rem;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    .card .value {
      margin-top: 10px;
      font-size: 2rem;
      font-weight: 700;
    }
    .panel {
      padding: 18px;
      margin-bottom: 16px;
    }
    .panel h2 {
      margin: 0 0 14px;
      font-size: 1.05rem;
      letter-spacing: 0.02em;
    }
    table {
      width: 100%;
      border-collapse: collapse;
    }
    th, td {
      text-align: left;
      padding: 10px 8px;
      border-bottom: 1px solid rgba(148, 163, 184, 0.14);
      vertical-align: top;
    }
    th {
      color: var(--muted);
      font-size: 0.8rem;
      text-transform: uppercase;
      letter-spacing: 0.08em;
    }
    tr:last-child td {
      border-bottom: 0;
    }
    .pill {
      display: inline-block;
      padding: 4px 10px;
      border-radius: 999px;
      background: rgba(56, 189, 248, 0.12);
      color: #c7f9ff;
    }
    .muted {
      color: var(--muted);
    }
    .good { color: var(--good); }
    .warn { color: var(--warn); }
    .bad { color: var(--bad); }
  </style>
</head>
<body>
  <div class="wrap">
    {{.Body}}
  </div>
</body>
</html>`))

func New(store storage.Storage) *Dashboard {
	return &Dashboard{engine: analytics.NewEngine(store)}
}

func (d *Dashboard) Handler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.overview)
	mux.HandleFunc("/tools", d.tools)
	mux.HandleFunc("/agents", d.agents)
	mux.HandleFunc("/errors", d.errors)
	return authMiddleware(token, mux)
}

func (d *Dashboard) Serve(ctx context.Context, addr, token string) error {
	if token == "" && !isLoopbackBind(addr) {
		return errors.New("dashboard token required for non-local bind address")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return d.ServeListener(ctx, ln, token)
}

func (d *Dashboard) ServeListener(ctx context.Context, ln net.Listener, token string) error {
	if ln == nil {
		return errors.New("dashboard listener is required")
	}
	if token == "" && !isLoopbackBind(ln.Addr().String()) {
		return errors.New("dashboard token required for non-local listener")
	}
	srv := &http.Server{
		Handler:           d.Handler(token),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func (d *Dashboard) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := d.engine.Stats(ctx, time.Time{})
	if err != nil {
		writeServerError(w, err)
		return
	}
	tools, err := d.engine.TopTools(ctx, time.Time{}, 5)
	if err != nil {
		writeServerError(w, err)
		return
	}
	agents, err := d.engine.TopAgents(ctx, time.Time{}, 5)
	if err != nil {
		writeServerError(w, err)
		return
	}
	failures, err := d.engine.ToolFailureRates(ctx, time.Time{}, 5)
	if err != nil {
		writeServerError(w, err)
		return
	}

	body := buildPageBody("Overview", "Operational snapshot and quick links.",
		[]string{"<a href=\"/tools\">Tools</a>", "<a href=\"/agents\">Agents</a>", "<a href=\"/errors\">Errors</a>"},
		[]string{
			metricCard("Calls", fmt.Sprintf("%d", stats.Calls)),
			metricCard("Success rate", fmt.Sprintf("%.1f%%", stats.SuccessRate*100)),
			metricCard("Avg latency", fmt.Sprintf("%.2f ms", stats.AvgLatency)),
			metricCard("Input size", fmt.Sprintf("%d", stats.InputSize)),
			metricCard("Output size", fmt.Sprintf("%d", stats.OutputSize)),
		},
		section("Top Tools", renderToolTable(tools)),
		section("Top Agents", renderAgentTable(agents)),
		section("High Failure Rate", renderFailureTable(failures)),
	)
	renderPage(w, "Overview", body)
}

func (d *Dashboard) tools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tools, err := d.engine.TopTools(ctx, time.Time{}, 20)
	if err != nil {
		writeServerError(w, err)
		return
	}
	failures, err := d.engine.ToolFailureRates(ctx, time.Time{}, 10)
	if err != nil {
		writeServerError(w, err)
		return
	}
	body := buildPageBody("Tools", "Tool usage and stability view.", []string{"<a href=\"/\">Overview</a>", "<a href=\"/agents\">Agents</a>", "<a href=\"/errors\">Errors</a>"},
		nil,
		section("Tool Heatmap", renderToolTable(tools)),
		section("Failure Rank", renderFailureTable(failures)),
	)
	renderPage(w, "Tools", body)
}

func (d *Dashboard) agents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agents, err := d.engine.TopAgents(ctx, time.Time{}, 20)
	if err != nil {
		writeServerError(w, err)
		return
	}
	eventsRows, err := d.engine.Store.ListEvents(ctx, maxInt())
	if err != nil {
		writeServerError(w, err)
		return
	}
	behavior := summarizeBehavior(eventsRows)
	body := buildPageBody("Agents", "Agent usage and behavior profile.", []string{"<a href=\"/\">Overview</a>", "<a href=\"/tools\">Tools</a>", "<a href=\"/errors\">Errors</a>"},
		[]string{
			metricCard("Profile", behavior.Label),
			metricCard("Read", fmt.Sprintf("%d", behavior.Read)),
			metricCard("Write", fmt.Sprintf("%d", behavior.Write)),
			metricCard("Other", fmt.Sprintf("%d", behavior.Other)),
		},
		section("Agent Usage", renderAgentTable(agents)),
		section("Behavior Profile", renderBehaviorSummary(behavior)),
	)
	renderPage(w, "Agents", body)
}

func (d *Dashboard) errors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	failures, err := d.engine.ToolFailureRates(ctx, time.Time{}, 20)
	if err != nil {
		writeServerError(w, err)
		return
	}
	breakdowns, err := d.engine.ErrorBreakdowns(ctx, time.Time{}, 20)
	if err != nil {
		writeServerError(w, err)
		return
	}
	eventsRows, err := d.engine.Store.ListEvents(ctx, maxInt())
	if err != nil {
		writeServerError(w, err)
		return
	}
	recentFailures := filterFailedEvents(eventsRows, 20)
	body := buildPageBody("Errors", "Failure analysis and recent failing calls.", []string{"<a href=\"/\">Overview</a>", "<a href=\"/tools\">Tools</a>", "<a href=\"/agents\">Agents</a>"},
		nil,
		section("Failure Rank", renderFailureTable(failures)),
		section("Failure Reasons", renderBreakdownTable(breakdowns)),
		section("Recent Failures", renderEventTable(recentFailures)),
	)
	renderPage(w, "Errors", body)
}

func renderPage(w http.ResponseWriter, title string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplate.Execute(w, pageData{Title: title, Body: body})
}

func buildPageBody(title, subtitle string, navLinks []string, cards []string, sections ...string) template.HTML {
	var b strings.Builder
	b.WriteString("<header class=\"hero\">")
	b.WriteString("<div>")
	b.WriteString("<span class=\"pill\">TraceAI Dashboard</span>")
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h1>")
	b.WriteString("<p>")
	b.WriteString(html.EscapeString(subtitle))
	b.WriteString("</p>")
	b.WriteString("</div>")
	b.WriteString("<nav class=\"nav\">")
	for _, link := range navLinks {
		b.WriteString(link)
	}
	b.WriteString("</nav></header>")
	if len(cards) > 0 {
		b.WriteString("<section class=\"grid\">")
		for _, card := range cards {
			b.WriteString(card)
		}
		b.WriteString("</section>")
	}
	for _, section := range sections {
		b.WriteString(section)
	}
	return template.HTML(b.String())
}

func metricCard(label, value string) string {
	return `<article class="card"><div class="label">` + html.EscapeString(label) + `</div><div class="value">` + html.EscapeString(value) + `</div></article>`
}

func section(title, body string) string {
	return `<section class="panel"><h2>` + html.EscapeString(title) + `</h2>` + body + `</section>`
}

func renderToolTable(rows []storage.ToolCount) string {
	if len(rows) == 0 {
		return `<p class="muted">No tool data yet.</p>`
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr><th>Tool</th><th>Calls</th><th>Success</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(row.ToolName))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Calls))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Success))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func renderAgentTable(rows []storage.AgentCount) string {
	if len(rows) == 0 {
		return `<p class="muted">No agent data yet.</p>`
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr><th>Agent</th><th>Calls</th><th>Success</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(row.AgentName))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Calls))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Success))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func renderFailureTable(rows []storage.ToolFailureRate) string {
	if len(rows) == 0 {
		return `<p class="muted">No failures yet.</p>`
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr><th>Tool</th><th>Calls</th><th>Failures</th><th>Rate</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(row.ToolName))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Calls))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Failures))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%.1f%%", row.FailureRate*100))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func renderBreakdownTable(rows []storage.ErrorBreakdown) string {
	if len(rows) == 0 {
		return `<p class="muted">No error breakdown yet.</p>`
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr><th>Category</th><th>Type</th><th>Code</th><th>Calls</th><th>Failures</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(row.Category))
		b.WriteString("</td><td>")
		if row.ErrorType == "" {
			b.WriteString("-")
		} else {
			b.WriteString(html.EscapeString(row.ErrorType))
		}
		b.WriteString("</td><td>")
		if row.ErrorCode == "" {
			b.WriteString("-")
		} else {
			b.WriteString(html.EscapeString(row.ErrorCode))
		}
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Calls))
		b.WriteString("</td><td>")
		b.WriteString(fmt.Sprintf("%d", row.Failures))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func renderBehaviorSummary(profile behaviorProfile) string {
	var b strings.Builder
	b.WriteString("<table><tbody>")
	rows := [][2]string{
		{"Profile", profile.Label},
		{"Read", fmt.Sprintf("%d", profile.Read)},
		{"Write", fmt.Sprintf("%d", profile.Write)},
		{"Other", fmt.Sprintf("%d", profile.Other)},
		{"Total", fmt.Sprintf("%d", profile.Total)},
	}
	for _, row := range rows {
		b.WriteString("<tr><th>")
		b.WriteString(html.EscapeString(row[0]))
		b.WriteString("</th><td>")
		b.WriteString(html.EscapeString(row[1]))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func renderEventTable(rows []events.ToolEvent) string {
	if len(rows) == 0 {
		return `<p class="muted">No recent failures.</p>`
	}
	var b strings.Builder
	b.WriteString("<table><thead><tr><th>Time</th><th>Agent</th><th>Tool</th><th>Function</th><th>Message</th></tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr><td>")
		b.WriteString(html.EscapeString(row.Timestamp.UTC().Format(time.RFC3339)))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(row.AgentName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(row.ToolName))
		b.WriteString("</td><td>")
		b.WriteString(html.EscapeString(row.FunctionName))
		b.WriteString("</td><td>")
		msg := row.ErrorMessage
		if msg == "" {
			msg = "n/a"
		}
		b.WriteString(html.EscapeString(msg))
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

func writeServerError(w http.ResponseWriter, err error) {
	slog.Default().With("component", "dashboard").Error("request failed", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func authMiddleware(token string, next http.Handler) http.Handler {
	if strings.TrimSpace(token) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="TraceAI Dashboard"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if tokenFromHeader(r, token) {
			http.SetCookie(w, &http.Cookie{
				Name:     dashboardTokenCookie,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
		}
		next.ServeHTTP(w, r)
	})
}

func authorized(r *http.Request, token string) bool {
	if strings.TrimSpace(token) == "" {
		return true
	}
	if cookie, err := r.Cookie(dashboardTokenCookie); err == nil && tokenMatches(cookie.Value, token) {
		return true
	}
	return tokenFromHeader(r, token)
}

func tokenFromHeader(r *http.Request, token string) bool {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		if tokenMatches(strings.TrimSpace(authorization[len("Bearer "):]), token) {
			return true
		}
	}
	if tokenMatches(strings.TrimSpace(r.Header.Get("X-TraceAI-Token")), token) {
		return true
	}
	return false
}

func tokenMatches(candidate, token string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1
}

func isLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func filterFailedEvents(rows []events.ToolEvent, limit int) []events.ToolEvent {
	failed := make([]events.ToolEvent, 0)
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Success {
			continue
		}
		failed = append(failed, rows[i])
	}
	sort.SliceStable(failed, func(i, j int) bool {
		return failed[i].Timestamp.After(failed[j].Timestamp)
	})
	if limit > 0 && len(failed) > limit {
		failed = failed[:limit]
	}
	return failed
}

func summarizeBehavior(rows []events.ToolEvent) behaviorProfile {
	var profile behaviorProfile
	for _, row := range rows {
		switch classifyFunction(row.FunctionName) {
		case "read":
			profile.Read++
		case "write":
			profile.Write++
		default:
			profile.Other++
		}
		profile.Total++
	}
	switch {
	case profile.Total == 0:
		profile.Label = "no-data"
	case profile.Read > profile.Write*2 && profile.Read >= profile.Other:
		profile.Label = "read-heavy"
	case profile.Write > profile.Read*2 && profile.Write >= profile.Other:
		profile.Label = "write-heavy"
	default:
		profile.Label = "balanced"
	}
	return profile
}

func classifyFunction(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case hasAnyPrefix(normalized, []string{"read", "get", "list", "search", "fetch", "find", "query"}):
		return "read"
	case hasAnyPrefix(normalized, []string{"write", "create", "update", "delete", "add", "remove", "patch", "merge", "commit", "insert", "edit"}):
		return "write"
	default:
		return "other"
	}
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
