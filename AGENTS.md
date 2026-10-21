# AGENTS.md

pangolin-site-monitor is one Go binary. It checks a Pangolin site and a list of Pangolin public resources on a cron schedule. It sends an email when a check result changes.

## Behavior

- The site check reads `GET /v1/org/{org}/site/{niceId}`. The site is good when `online` is true.
- The resource check reads `GET /v1/org/{org}/public-resources`. A resource is good when each of its targets has `healthStatus` `healthy`.
- The monitor sends an alert email after 2 bad checks in a row.
- The monitor sends a recovery email after the first good check that follows an alert.
- If an email fails, the monitor keeps the previous state. The next check tries the email again.
- Inside `QUIET_WINDOW`, the monitor does no checks and does not change the state.
- The state is in memory only. A restart clears it.

## Layout

- `main.go`: all code, in package `main`.
- `main_test.go`: the tests.
- `Dockerfile`: the build stage and the distroless runtime image.
- `docker-compose.yaml`: an example for a local run.
- `README.md`: the reference for all environment variables. Do not copy that table into this file.

## Commands

Use these commands to check a change:

```sh
gofmt -l .
go vet ./...
go test ./...
docker build .
```

If Go is not installed, run the first three commands in the `golang` image that the `Dockerfile` uses.

## Deploy

1. The homelab role `pangolin-site-monitor` deploys this service on `pangolin-vm`. The role is in `~/personal/homelab/ansible/roles/pangolin-site-monitor`.
2. The role clones `HEAD` of this repository from GitHub. Push a change before you deploy it.
3. Start the deploy with the automation-client job `deploy-pangolin-site-monitor`.
4. The values for the environment variables are in the role defaults and in `services.pangolin_site_monitor` in the homelab `group_vars/all.yaml`.

## Rules

- Use the Go standard library. The only dependency is `github.com/robfig/cron/v3`. Do not add a dependency.
- Keep all code in `main.go`. Do not add a package.
- Write a test in `main_test.go` for each parser and each health rule.
- When you add or change an environment variable, update `README.md`, `docker-compose.yaml` and the homelab role template.
- Write no comment by default. Write a comment only for a constraint that the code cannot show.
- Do not commit and do not push. The owner does this.
