# Pangolin Site Monitor

A minimal Go service that checks a Pangolin Integration API endpoint on a cron schedule and sends an email alert when:

- the site/tunnel is **offline** (`data.online == false`), or
- the API call **fails** (timeout, non-200, invalid JSON, etc.).

## Configuration (Environment Variables)

| Variable | Required | Example | Notes |
|---|---|---|---|
| `PANGOLIN_INT_API_PROTOCOL` | yes | `https` | `http` or `https` |
| `PANGOLIN_INT_API_HOSTNAME` | yes | `host.docker.internal` | Hostname of the Integration API |
| `PANGOLIN_INT_API_PORT` | yes | `443` | API port |
| `PANGOLIN_INT_API_TOKEN` | yes | `abc123` | API token with get and list permissions for org and site |
| `PANGOLIN_ORG_ID` | yes | `abc123` | Your Pangolin organization id |
| `PANGOLIN_SITE_NICE_ID` | yes | `homelab` | The site nice id |
| `CRON_SCHEDULE` | yes | `*/5 * * * *` | Five-field cron spec (minute hour dom month dow), **UTC** |
| `SMTP_USER` | yes | `alerts@example.com` | Used as SMTP auth username and email sender |
| `SMTP_PASSWORD` | yes | `app_pw` | SMTP password / app password |
| `SMTP_SERVER` | yes | `smtp.example.com` | SMTP server hostname |
| `SMTP_PORT` | yes | `587` | `587` (STARTTLS) or `465` (TLS) supported |
| `RECIPIENT_EMAIL` | yes | `you@example.com` | Where alerts are sent |