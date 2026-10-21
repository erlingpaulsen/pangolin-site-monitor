# Pangolin Site Monitor

A minimal Go service that checks a Pangolin Integration API endpoint on a cron schedule and sends an email alert when:

- the site/tunnel is **offline** (`data.online == false`),
- a watched public resource is **unhealthy** (any of its targets has a health check status other than `healthy`), or
- an API call **fails** (timeout, non-200, invalid JSON, resource not found, etc.).

An alert goes out after two bad checks in a row; a recovery email goes out on the first good check after an alert. Checks are skipped inside the optional quiet window.

## Configuration (Environment Variables)

| Variable | Required | Example | Notes |
|---|---|---|---|
| `PANGOLIN_INT_API_PROTOCOL` | yes | `https` | `http` or `https` |
| `PANGOLIN_INT_API_HOSTNAME` | yes | `localhost` | Hostname of the Integration API |
| `PANGOLIN_INT_API_PORT` | yes | `443` | API port |
| `PANGOLIN_INT_API_TOKEN` | yes | `abc123` | API token with getSite, and listResources when `PANGOLIN_RESOURCE_DOMAINS` is set |
| `PANGOLIN_ORG_ID` | yes | `abc123` | Your Pangolin organization id |
| `PANGOLIN_SITE_NICE_ID` | yes | `homelab` | The site nice id |
| `PANGOLIN_RESOURCE_DOMAINS` | no | `app.example.com,api.example.com` | Comma-separated full domains of public resources to watch |
| `CRON_SCHEDULE` | yes | `*/5 * * * *` | Five-field cron spec (minute hour dom month dow), in `TZ` |
| `QUIET_WINDOW` | no | `00:55-02:00` | `HH:MM-HH:MM` in `TZ`, may cross midnight; no checks run inside it |
| `TZ` | no | `Europe/Oslo` | Time zone for `CRON_SCHEDULE`, `QUIET_WINDOW` and logs; default UTC |
| `SMTP_USER` | yes | `alerts@example.com` | Used as SMTP auth username and email sender |
| `SMTP_PASSWORD` | yes | `app_pw` | SMTP password / app password |
| `SMTP_SERVER` | yes | `smtp.example.com` | SMTP server hostname |
| `SMTP_PORT` | yes | `587` | `587` (STARTTLS) or `465` (TLS) supported |
| `RECIPIENT_EMAIL` | yes | `you@example.com` | Where alerts are sent |
