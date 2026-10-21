package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

// ----- Config -----

type Config struct {
	Protocol        string
	Host            string
	Port            string
	OrgID           string
	SiteNiceID      string
	CronSpec        string
	SMTPUser        string
	SMTPPass        string
	SMTPServer      string
	SMTPPort        string
	Recipient       string
	Token           string
	ResourceDomains []string
	QuietWindow     *quietWindow
	schedule        cron.Schedule
}

func getEnv(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func loadConfig() (Config, error) {
	var cfg Config
	var missing []string
	for _, v := range []struct {
		key string
		dst *string
	}{
		{"PANGOLIN_INT_API_PROTOCOL", &cfg.Protocol},
		{"PANGOLIN_INT_API_HOSTNAME", &cfg.Host},
		{"PANGOLIN_INT_API_PORT", &cfg.Port},
		{"PANGOLIN_INT_API_TOKEN", &cfg.Token},
		{"PANGOLIN_ORG_ID", &cfg.OrgID},
		{"PANGOLIN_SITE_NICE_ID", &cfg.SiteNiceID},
		{"CRON_SCHEDULE", &cfg.CronSpec},
		{"SMTP_USER", &cfg.SMTPUser},
		{"SMTP_PASSWORD", &cfg.SMTPPass},
		{"SMTP_SERVER", &cfg.SMTPServer},
		{"SMTP_PORT", &cfg.SMTPPort},
		{"RECIPIENT_EMAIL", &cfg.Recipient},
	} {
		*v.dst = getEnv(v.key)
		if *v.dst == "" {
			missing = append(missing, v.key)
		}
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}

	for _, d := range strings.Split(getEnv("PANGOLIN_RESOURCE_DOMAINS"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			cfg.ResourceDomains = append(cfg.ResourceDomains, d)
		}
	}

	var err error
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if cfg.schedule, err = parser.Parse(cfg.CronSpec); err != nil {
		return cfg, fmt.Errorf("invalid CRON_SCHEDULE: %w", err)
	}
	if cfg.QuietWindow, err = parseQuietWindow(getEnv("QUIET_WINDOW")); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) apiBase() string {
	return fmt.Sprintf("%s://%s:%s/v1/org/%s", c.Protocol, c.Host, c.Port, c.OrgID)
}

// ----- Quiet window -----

type quietWindow struct {
	spec       string
	start, end int
}

func parseQuietWindow(s string) (*quietWindow, error) {
	if s == "" {
		return nil, nil
	}
	a, b, _ := strings.Cut(s, "-")
	start, errStart := time.Parse("15:04", a)
	end, errEnd := time.Parse("15:04", b)
	if errStart != nil || errEnd != nil {
		return nil, fmt.Errorf("invalid QUIET_WINDOW %q, want HH:MM-HH:MM", s)
	}
	return &quietWindow{spec: s, start: start.Hour()*60 + start.Minute(), end: end.Hour()*60 + end.Minute()}, nil
}

func (w *quietWindow) contains(t time.Time) bool {
	if w == nil {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if w.start <= w.end {
		return m >= w.start && m < w.end
	}
	return m >= w.start || m < w.end
}

// ----- Pangolin API -----

var httpClient = &http.Client{Timeout: 10 * time.Second}

func getJSON(ctx context.Context, url, token string, data any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Status  int             `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if !envelope.Success || envelope.Status != http.StatusOK {
		return fmt.Errorf("api indicated failure: success=%v status=%d message=%s", envelope.Success, envelope.Status, envelope.Message)
	}
	return json.Unmarshal(envelope.Data, data)
}

type result struct {
	key    string
	label  string
	ok     bool
	status string
	detail string
}

func checkSite(ctx context.Context, cfg Config) result {
	url := fmt.Sprintf("%s/site/%s", cfg.apiBase(), cfg.SiteNiceID)
	r := result{key: "site", label: "Site " + cfg.SiteNiceID}

	var site struct {
		Online  bool   `json:"online"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := getJSON(ctx, url, cfg.Token, &site); err != nil {
		r.status, r.detail = "FAILED", fmt.Sprintf("Endpoint: %s\nError: %v\n", url, err)
		return r
	}
	if site.Name != "" {
		r.label = "Site " + site.Name
	}
	r.ok, r.status = site.Online, "OFFLINE"
	if site.Online {
		r.status = "ONLINE"
	}
	r.detail = fmt.Sprintf("Endpoint: %s\nOrg: %s\nSite: %s\nMessage: %s\n", url, cfg.OrgID, cfg.SiteNiceID, site.Message)
	return r
}

type resource struct {
	FullDomain string `json:"fullDomain"`
	Targets    []struct {
		HealthStatus string `json:"healthStatus"`
	} `json:"targets"`
}

func (r resource) healthy() bool {
	for _, t := range r.Targets {
		if t.HealthStatus != "healthy" {
			return false
		}
	}
	return len(r.Targets) > 0
}

func checkResources(ctx context.Context, cfg Config) []result {
	if len(cfg.ResourceDomains) == 0 {
		return nil
	}
	url := cfg.apiBase() + "/public-resources?pageSize=1000"
	var list struct {
		Resources []resource `json:"resources"`
	}
	err := getJSON(ctx, url, cfg.Token, &list)

	results := make([]result, 0, len(cfg.ResourceDomains))
	for _, domain := range cfg.ResourceDomains {
		r := result{key: domain, label: "Resource " + domain, status: "FAILED"}
		switch i := indexByDomain(list.Resources, domain); {
		case err != nil:
			r.detail = fmt.Sprintf("Endpoint: %s\nError: %v\n", url, err)
		case i < 0:
			r.detail = fmt.Sprintf("Endpoint: %s\nError: no resource with fullDomain %s\n", url, domain)
		default:
			res := list.Resources[i]
			r.ok, r.status = res.healthy(), "UNHEALTHY"
			if r.ok {
				r.status = "HEALTHY"
			}
			var targets []string
			for _, t := range res.Targets {
				targets = append(targets, t.HealthStatus)
			}
			r.detail = fmt.Sprintf("Endpoint: %s\nTarget health: %s\n", url, strings.Join(targets, ", "))
		}
		results = append(results, r)
	}
	return results
}

func indexByDomain(resources []resource, domain string) int {
	for i, r := range resources {
		if r.FullDomain == domain {
			return i
		}
	}
	return -1
}

// ----- Email -----

func dialSMTP(cfg Config) (*smtp.Client, error) {
	addr := net.JoinHostPort(cfg.SMTPServer, cfg.SMTPPort)
	tlsCfg := &tls.Config{ServerName: cfg.SMTPServer}
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	var err error
	if cfg.SMTPPort == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	client, err := smtp.NewClient(conn, cfg.SMTPServer)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if cfg.SMTPPort != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				client.Close()
				return nil, err
			}
		}
	}
	return client, nil
}

func sendEmail(cfg Config, subject, body string) error {
	client, err := dialSMTP(cfg)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPServer)); err != nil {
			return err
		}
	}
	if err := client.Mail(cfg.SMTPUser); err != nil {
		return err
	}
	if err := client.Rcpt(cfg.Recipient); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	msg := "From: " + cfg.SMTPUser + "\n" +
		"To: " + cfg.Recipient + "\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\n" +
		"MIME-Version: 1.0\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\n\n" +
		body
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// ----- Alert state -----

const badChecksBeforeAlert = 2

type checkState struct {
	bad     int
	alerted bool
	status  string
}

var states = map[string]*checkState{}

func report(cfg Config, r result) {
	st := states[r.key]
	if st == nil {
		st = &checkState{}
		states[r.key] = st
	}
	body := fmt.Sprintf("Time: %s\nCheck: %s\nStatus: %s\n%s", time.Now().Format(time.RFC3339), r.label, r.status, r.detail)

	if r.ok {
		st.bad = 0
		if !st.alerted {
			log.Printf("OK - %s is %s", r.label, r.status)
			return
		}
		log.Printf("RECOVERY - %s is %s (previously %s). Sending email.", r.label, r.status, st.status)
		if err := sendEmail(cfg, fmt.Sprintf("🟢[Pangolin Monitor] %s is %s (recovered)", r.label, r.status), body); err != nil {
			log.Printf("ERROR - Recovery email for %s failed, retrying next check: %v", r.label, err)
			return
		}
		st.alerted = false
		return
	}

	st.bad++
	switch {
	case st.alerted && st.status == r.status:
		log.Printf("WARN - %s is %s (already alerted). %s", r.label, r.status, oneLine(r.detail))
		return
	case st.bad < badChecksBeforeAlert:
		log.Printf("WARN - %s is %s (%d/%d before alert). %s", r.label, r.status, st.bad, badChecksBeforeAlert, oneLine(r.detail))
		return
	}

	subject := fmt.Sprintf("🚨[Pangolin Monitor] %s is %s", r.label, r.status)
	if r.status == "FAILED" {
		subject = fmt.Sprintf("⚠️[Pangolin Monitor] %s check FAILED", r.label)
	}
	log.Printf("ALERT - %s is %s. Sending email. %s", r.label, r.status, oneLine(r.detail))
	if err := sendEmail(cfg, subject, body); err != nil {
		log.Printf("ERROR - Alert email for %s failed, retrying next check: %v", r.label, err)
		return
	}
	st.alerted, st.status = true, r.status
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "; ")
}

// ----- Monitor job -----

func runCheck(cfg Config) {
	if cfg.QuietWindow.contains(time.Now()) {
		log.Printf("INFO - Inside QUIET_WINDOW %s, skipping checks.", cfg.QuietWindow.spec)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	report(cfg, checkSite(ctx, cfg))
	for _, r := range checkResources(ctx, cfg) {
		report(cfg, r)
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	quiet := "none"
	if cfg.QuietWindow != nil {
		quiet = cfg.QuietWindow.spec
	}
	log.Printf("INFO - Starting pangolin-site-monitor. api=%s site=%s resources=%v schedule='%s' quiet=%s tz=%s",
		cfg.apiBase(), cfg.SiteNiceID, cfg.ResourceDomains, cfg.CronSpec, quiet, time.Local)

	runCheck(cfg)

	c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	c.Schedule(cfg.schedule, cron.FuncJob(func() { runCheck(cfg) }))
	c.Run()
}
