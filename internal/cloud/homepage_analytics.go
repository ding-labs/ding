package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const homepageAnalyticsOrigin = "https://ding.ing"

var homepageHandoff = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")

type homepageAnalytics struct {
	key    string
	origin string
	client *http.Client
}

// Only the public-site Worker receives this dedicated credential. Redirects
// must never carry credentials or a pseudonymous account to another origin.
func newHomepageAnalytics(key string) *homepageAnalytics {
	return &homepageAnalytics{key: key, origin: homepageAnalyticsOrigin, client: &http.Client{
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (a *homepageAnalytics) post(ctx context.Context, action, subject, handoff string) (*http.Response, error) {
	account := sha256.Sum256([]byte(subject))
	payload, err := json.Marshal(map[string]string{"action": action, "account": hex.EncodeToString(account[:]), "handoff": handoff})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.origin+"/_funnel/"+action, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(a.key))
	io.WriteString(mac, ts+".")
	mac.Write(payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Ding Cloud homepage analytics/1.0")
	req.Header.Set("Ding-Signature", "t="+ts+", v1="+hex.EncodeToString(mac.Sum(nil)))
	return a.client.Do(req)
}

func (a *homepageAnalytics) confirm(ctx context.Context, subject, handoff string) error {
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := a.post(ctx, "verified", subject, handoff)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusNoContent {
				return nil
			}
			if resp.StatusCode < 500 {
				return fmt.Errorf("receipt returned %d", resp.StatusCode)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return errors.New("homepage measurement unavailable")
}

type githubDownloads struct {
	Observed int64
	Baseline int64
	Cutoff   int64
	Archives int
	Bottles  int
	Excluded int
}

func (g githubDownloads) Partial() bool {
	return time.Unix(g.Baseline, 0).UTC().Truncate(24 * time.Hour).After(time.Unix(g.Cutoff, 0))
}

type brewDownloads struct {
	Observed int64
	Start    string `json:"start_date"`
	End      string `json:"end_date"`
	Installs int
}

var localDownloadsReport = template.Must(template.New("downloads").Funcs(template.FuncMap{
	"utc": func(t int64) string { return time.Unix(t, 0).UTC().Format("Jan 2, 2006 at 15:04 UTC") },
	"day": func(t int64) string { return time.Unix(t, 0).UTC().Format("Jan 2, 2006") },
}).Parse(`
<section><h2>Trying Ding locally</h2>
<p>Download counts refresh daily, scheduled for 04:17 UTC. A Homebrew install can also download a GitHub file, and one person can download several copies.</p>
<table cellpadding="8"><thead><tr><th scope="col">Activity</th><th scope="col">Count</th></tr></thead><tbody>
<tr><th scope="row">GitHub · native archive downloads</th><td>{{with .GitHub}}{{.Archives}}{{else}}Pending{{end}}</td></tr>
<tr><th scope="row">GitHub · Homebrew bottle downloads</th><td>{{with .GitHub}}{{.Bottles}}{{else}}Pending{{end}}</td></tr>
<tr><th scope="row">Homebrew · reported installs on request</th><td>{{with .Homebrew}}{{.Installs}}{{else}}Pending{{end}}</td></tr></tbody></table>
{{with .GitHub}}<p><strong>GitHub</strong> · Updated {{utc .Observed}}. {{if .Partial}}Partial 30-day history. Files uploaded since {{day .Cutoff}} contribute their recorded downloads. For older files, only increases observed since {{utc .Baseline}} are available.{{else}}Counts cover changes since {{utc .Baseline}}.{{end}} Excluded {{.Excluded}} identified bottle-build fetch{{if ne .Excluded 1}}es{{end}}. Other automation, repeat downloads and personal downloads can still appear.</p>{{end}}
{{with .Homebrew}}<p><strong>Homebrew</strong> · Published period {{.Start}} through {{.End}}. Checked {{utc .Observed}}. Counts explicit installs of <code>ding-labs/tap/ding</code> reported to Homebrew. Our bottle jobs disable Homebrew analytics. A zero means Homebrew listed no installs for Ding.</p>{{end}}
</section></main></html>`))

type homepageCounts struct {
	GitHub   *githubDownloads `json:"github"`
	Homebrew *brewDownloads   `json:"homebrew"`
	Landings int              `json:"homepage_browsers"`
	Clicks   int              `json:"browsers_clicked_get_started"`
	Signups  int              `json:"signed_up_from_button"`
	Total    int              `json:"all_outside_signups"`
}

func (s *Server) homepageReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	if r.Method != "GET" || s.Login.homepage == nil {
		http.NotFound(w, r)
		return
	}
	session, err := s.authenticate(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?return=/analytics/homepage", http.StatusFound)
		return
	}
	if session.Kind != "browser" {
		http.Error(w, "Private report", http.StatusForbidden)
		return
	}
	subjects, err := s.DB.IdentitySubjects(r.Context(), session.User, s.AuthKitIssuer)
	if err != nil {
		http.Error(w, "Analytics temporarily unavailable", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var allowed *http.Response
	for _, subject := range subjects {
		resp, err := s.Login.homepage.post(ctx, "report", subject, "")
		if err != nil {
			http.Error(w, "Analytics temporarily unavailable", 503)
			return
		}
		if resp.StatusCode != http.StatusForbidden {
			allowed = resp
			break
		}
		resp.Body.Close()
	}
	if allowed == nil {
		http.Error(w, "Private report", http.StatusForbidden)
		return
	}
	defer allowed.Body.Close()
	var counts homepageCounts
	if allowed.StatusCode != 200 || json.NewDecoder(io.LimitReader(allowed.Body, 4096)).Decode(&counts) != nil {
		http.Error(w, "Analytics temporarily unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Homepage funnel · Ding Cloud</title>
<style>
:root{color-scheme:light dark;font-family:system-ui,sans-serif}body{margin:0}main{max-width:46rem;margin:clamp(2rem,7vw,5rem) auto;padding:0 1.25rem}h1{font-size:clamp(1.8rem,6vw,3rem);letter-spacing:-.04em;margin-bottom:.4rem}p{line-height:1.55}table{width:100%%;border-collapse:collapse;margin:2rem 0}th,td{border-bottom:1px solid #8888;text-align:left;padding:.85rem .3rem}thead th{font-size:.85rem}tbody th{font-weight:500}td,thead th:last-child{text-align:right;font-variant-numeric:tabular-nums}td{font-size:1.5rem;font-weight:650}
</style><main><h1>Ding homepage funnel</h1><p>Last 30 UTC calendar days, including today. Refresh this page for current funnel counts. Owner accounts and recognized owner browsers are excluded.</p>
<table cellpadding="8"><thead><tr><th scope="col">Step</th><th scope="col">Count</th></tr></thead><tbody>
<tr><th scope="row">Browsers that landed on ding.ing</th><td>%d</td></tr>
<tr><th scope="row">Browsers that clicked Get started with Ding Cloud</th><td>%d</td></tr>
<tr><th scope="row">New accounts that completed signup through that button</th><td>%d</td></tr></tbody></table>
<p>New accounts across all signup paths: <strong>%d</strong>.</p>
<p>Repeat visits and clicks count once per browser. Button signups require a new WorkOS account and a verified return from the same login flow. Visits and clicks started being measured November 9, 2026.</p>
<p><a href="https://ding.ing/analytics/exclude">Exclude another browser you use</a></p>`, counts.Landings, counts.Clicks, counts.Signups, counts.Total)
	_ = localDownloadsReport.Execute(w, counts)
}
