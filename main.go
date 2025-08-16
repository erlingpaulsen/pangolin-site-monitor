package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ----- Config -----

type Config struct {
	Protocol   string
	Host       string
	Port       string
	OrgID      string
	SiteNiceID string
	CronSpec   string
	SMTPUser   string
	SMTPPass   string
	SMTPServer string
	SMTPPort   string
	Recipient  string
}

func getEnv(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func loadConfig() (Config, error) {
	cfg := Config{
		Protocol:   getEnv("PANGOLIN_INT_API_PROTOCOL"),
		Host:       getEnv("PANGOLIN_INT_API_HOSTNAME"),
		Port:       getEnv("PANGOLIN_INT_API_PORT"),
		OrgID:      getEnv("PANGOLIN_ORG_ID"),
		SiteNiceID: getEnv("PANGOLIN_SITE_NICE_ID"),
		CronSpec:   getEnv("CRON_SCHEDULE"),
		SMTPUser:   getEnv("SMTP_USER"),
		SMTPPass:   getEnv("SMTP_PASSWORD"),
		SMTPServer: getEnv("SMTP_SERVER"),
		SMTPPort:   getEnv("SMTP_PORT"),
		Recipient:  getEnv("RECIPIENT_EMAIL"),
	}
	missing := []string{}
	if cfg.Protocol == "" {
		missing = append(missing, "PANGOLIN_INT_API_PROTOCOL")
	}
	if cfg.Host == "" {
		missing = append(missing, "PANGOLIN_INT_API_HOSTNAME")
	}
	if cfg.Port == "" {
		missing = append(missing, "PANGOLIN_INT_API_PORT")
	}
	if cfg.OrgID == "" {
		missing = append(missing, "PANGOLIN_ORG_ID")
	}
	if cfg.SiteNiceID == "" {
		missing = append(missing, "PANGOLIN_SITE_NICE_ID")
	}
	if cfg.CronSpec == "" {
		missing = append(missing, "CRON_SCHEDULE")
	}
	if cfg.SMTPUser == "" {
		missing = append(missing, "SMTP_USER")
	}
	if cfg.SMTPPass == "" {
		missing = append(missing, "SMTP_PASSWORD")
	}
	if cfg.SMTPServer == "" {
		missing = append(missing, "SMTP_SERVER")
	}
	if cfg.SMTPPort == "" {
		missing = append(missing, "SMTP_PORT")
	}
	if cfg.Recipient == "" {
		missing = append(missing, "RECIPIENT_EMAIL")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func (c Config) endpoint() string {
	return fmt.Sprintf("%s://%s:%s/v1/org/%s/%s", c.Protocol, c.Host, c.Port, c.OrgID, c.SiteNiceID)
}

// ----- HTTP & payload -----

type siteData struct {
	Online  bool   `json:"online"`
	Name    string `json:"name"`
	NiceID  string `json:"niceId"`
	OrgID   string `json:"orgId"`
	Message string `json:"message"`
}

type siteResponse struct {
	Data    siteData `json:"data"`
	Success bool     `json:"success"`
	Error   bool     `json:"error"`
	Message string   `json:"message"`
	Status  int      `json:"status"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func checkAPI(ctx context.Context, url string) (siteResponse, error) {
	var respObj siteResponse

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return respObj, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return respObj, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return respObj, fmt.Errorf("unexpected status: %s", resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&respObj); err != nil {
		return respObj, err
	}
	// Basic sanity
	if !respObj.Success || respObj.Status != 200 {
		return respObj, fmt.Errorf("api indicated failure: success=%v status=%d message=%s", respObj.Success, respObj.Status, respObj.Message)
	}
	return respObj, nil
}

// ----- Email -----

type smtpCfg struct {
	User      string
	Pass      string
	Server    string
	Port      string
	Recipient string
}

func sendEmail(c smtpCfg, subject, body string) error {
	if c.User == "" || c.Pass == "" || c.Server == "" || c.Port == "" || c.Recipient == "" {
		return errors.New("smtp config incomplete")
	}

	from := c.User
	to := []string{c.Recipient}
	addr := net.JoinHostPort(c.Server, c.Port)
	host := c.Server

	// Note: Corrected email message formatting for clarity and standards.
	msg := "From: " + from + "\n" +
		"To: " + c.Recipient + "\n" +
		"Subject: " + subject + "\n" +
		"MIME-Version: 1.0\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\n\n" +
		body

	auth := smtp.PlainAuth("", c.User, c.Pass, host)

	// If port == 465, do implicit TLS; otherwise attempt STARTTLS
	if c.Port == "465" {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer client.Close()
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		for _, rcpt := range to {
			if err := client.Rcpt(rcpt); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return client.Quit()
	}

	// STARTTLS path
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()
	// Try STARTTLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return err
		}
	}
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// ----- In-memory state (alert de-dup) -----

type monitorState struct {
	last string // "unknown", "online", "offline", "api_error"
	mu   sync.Mutex
}
var state = monitorState{last: "unknown"}

// ----- Monitor job -----

func runCheck(cfg Config) {
	url := cfg.endpoint()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Determine current state
	current := "online"
	res, err := checkAPI(ctx, url)
	if err != nil {
		current = "api_error"
	} else if !res.Data.Online {
		current = "offline"
	}

	// Critical section for reading/updating last state
	state.mu.Lock()
	prev := state.last
	state.last = current
	state.mu.Unlock()

	// Handle logging and notifications
	switch current {
	case "api_error":
		if prev != current { // transition into API error
			log.Printf("API CHECK FAILED (prev=%s)", prev)
			_ = sendEmail(smtpCfg{User: cfg.SMTPUser, Pass: cfg.SMTPPass, Server: cfg.SMTPServer, Port: cfg.SMTPPort, Recipient: cfg.Recipient},
				"[Pangolin Monitor] API check FAILED",
				fmt.Sprintf("Time (UTC): %s\nEndpoint: %s\nError: %v\n", time.Now().UTC().Format(time.RFC3339), url, err))
		} else {
			log.Printf("API CHECK FAILED (unchanged, suppressing repeat email)")
		}
		return

	case "offline":
		if prev != current { // transition into offline
			name := res.Data.Name
			if name == "" {
				name = cfg.SiteNiceID
			}
			log.Printf("SITE OFFLINE: %s (%s) (prev=%s)", name, cfg.SiteNiceID, prev)
			subj := fmt.Sprintf("[Pangolin Monitor] Site %s is OFFLINE", name)
			body := fmt.Sprintf("Time (UTC): %s\nEndpoint: %s\nOrg: %s\nSite: %s\nOnline: %v\nMessage: %s\n", time.Now().UTC().Format(time.RFC3339), url, cfg.OrgID, cfg.SiteNiceID, res.Data.Online, res.Data.Message)
			_ = sendEmail(smtpCfg{User: cfg.SMTPUser, Pass: cfg.SMTPPass, Server: cfg.SMTPServer, Port: cfg.SMTPPort, Recipient: cfg.Recipient}, subj, body)
		} else {
			log.Printf("SITE OFFLINE (unchanged, suppressing repeat email)")
		}
		return

	default: // online
		if prev == "offline" || prev == "api_error" {
			log.Printf("RECOVERY: site back ONLINE (prev=%s)", prev)
			name := res.Data.Name
			if name == "" {
				name = cfg.SiteNiceID
			}
			subj := fmt.Sprintf("[Pangolin Monitor] Site %s is ONLINE (recovered)", name)
			body := fmt.Sprintf("Time (UTC): %s\nEndpoint: %s\nOrg: %s\nSite: %s\nPrevious state: %s\n", time.Now().UTC().Format(time.RFC3339), url, cfg.OrgID, cfg.SiteNiceID, prev)
			_ = sendEmail(smtpCfg{User: cfg.SMTPUser, Pass: cfg.SMTPPass, Server: cfg.SMTPServer, Port: cfg.SMTPPort, Recipient: cfg.Recipient}, subj, body)
		} else {
			log.Printf("OK: site online (no change)")
		}
	}
}

func main() {
	log.SetFlags(0)
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	log.Printf("starting pangolin-site-monitor | endpoint=%s | schedule=%s (UTC)", cfg.endpoint(), cfg.CronSpec)

	// Run once on startup
	runCheck(cfg)

	// Prepare 5-field cron (min,hour,dom,month,dow)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cfg.CronSpec)
	if err != nil {
		log.Fatalf("invalid CRON_SCHEDULE: %v", err)
	}

	c := cron.New(cron.WithParser(parser))
	c.Schedule(sched, cron.FuncJob(func() { runCheck(cfg) }))

	c.Run()
}