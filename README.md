<p align="center">
  <img src="logo.svg" alt="RedaktFlow" width="320">
</p>

<h1 align="center">RedaktFlow</h1>
<p align="center">AI content operations engine for Make.com, n8n, and Notion.</p>

<p align="center">
  <a href="https://github.com/renezander030/redaktflow/stargazers"><img src="https://img.shields.io/github/stars/renezander030/redaktflow?style=flat-square" alt="Stars"></a>
  <a href="https://github.com/renezander030/redaktflow/blob/master/LICENSE"><img src="https://img.shields.io/github/license/renezander030/redaktflow?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go" alt="Go 1.24">
  <img src="https://img.shields.io/badge/build-passing-brightgreen?style=flat-square" alt="Build">
</p>

RedaktFlow monitors, optimizes, and orchestrates your content automation workflows. It connects to Make.com, n8n, and Notion, adds AI-powered content drafting and repurposing, and keeps a human in the loop before anything publishes.

Runs as a CLI tool on your laptop or as a daemon on a server. No VPS required.

**Your workflows run on rules. This adds editorial judgment.**

## Quickstart

```bash
git clone https://github.com/renezander030/redaktflow.git && cd redaktflow
cp secrets.yaml.example secrets.yaml   # add your API keys
go build -o redaktflow .
```

Three modes of operation:

```bash
# CLI — agent-callable subcommands, JSON output
./redaktflow notion list-dbs
./redaktflow make list-scenarios
./redaktflow status

# One-shot — run a single pipeline and exit
./redaktflow --run scenario-health

# Daemon — scheduled pipelines on a loop
./redaktflow
```

## CLI Reference

Agent-first interface. Every command returns structured JSON. Use from scripts, Claude Code, or any agent harness.

```bash
# Notion
./redaktflow notion list-dbs
./redaktflow notion get-db --id <database_id>
./redaktflow notion query --db <database_id> --filter '{"property":"Status","select":{"equals":"Draft"}}'
./redaktflow notion create-db --parent <page_id> --name "Content Calendar" --props '{"Status":"select","Platform":"select","Date":"date"}'
./redaktflow notion create-page --db <database_id> --props '{"Name":{"title":[{"text":{"content":"Post #1"}}]}}'
./redaktflow notion update-page --id <page_id> --props '{"Status":{"select":{"name":"Published"}}}'

# Make.com
./redaktflow make list-scenarios
./redaktflow make get-scenario --id <scenario_id>
./redaktflow make get-blueprint --id <scenario_id>
./redaktflow make set-blueprint --id <scenario_id> --file blueprint.json
./redaktflow make create-scenario --name "Content Pipeline" --blueprint blueprint.json
./redaktflow make run --id <scenario_id>
./redaktflow make executions --id <scenario_id> --status error --limit 5
./redaktflow make activate --id <scenario_id>

# n8n
./redaktflow n8n list-workflows
./redaktflow n8n get-workflow --id <workflow_id>
./redaktflow n8n executions --status error --limit 10
./redaktflow n8n retry --id <execution_id>
./redaktflow n8n activate --id <workflow_id>

# Status — connection health for all platforms
./redaktflow status

# Help
./redaktflow help
```

All commands return JSON:
```json
{
  "ok": true,
  "command": "notion list-dbs",
  "data": [...],
  "count": 3
}
```

Define your pipelines in `config.yaml`, your prompts in `skills/`, and RedaktFlow handles the rest.

## What It Does

| Capability | How it works |
|---|---|
| **Monitor** | Watch Make.com scenarios and n8n workflows for failures, alert with diagnostics |
| **Optimize** | AI analyzes workflow blueprints for redundancy, error handling gaps, cost waste |
| **Draft** | Generate platform-specific content (LinkedIn, Dev.to, Twitter/X, Hashnode) from topics |
| **Repurpose** | Transform content across platforms -- re-angle, don't just resize |
| **Schedule** | AI-planned content calendar across platforms with topic spreading |
| **Approve** | Every piece of content goes through human review before publishing |

## Integrations

| Platform | What RedaktFlow does |
|---|---|
| **Make.com** | List/create/update scenarios, monitor executions, read/write blueprints, trigger runs |
| **n8n** | List/create/update workflows, monitor executions, retry failures, audit nodes |
| **Notion** | Query databases, create/update pages, sync content calendars, manage structured data |
| **Slack / Telegram** | Operator approval channel (human-in-the-loop) |

Adding a new integration means writing one Go file. Each connector follows the same pattern: fetch data, classify with AI, draft output, get human approval.

## How It Works

RedaktFlow runs pipelines. Each pipeline is a sequence of typed steps:

| Step type | What it does |
|---|---|
| `deterministic` | Plain code: fetch scenarios, check executions, pull workflows |
| `ai` | LLM inference with a skill template, budget-checked |
| `approval` | Human-in-the-loop: operator reviews before proceeding |

Example pipelines:

```yaml
pipelines:
  # Alert on failed Make.com scenarios with AI diagnosis
  - name: scenario-health
    schedule: 1h
    steps:
      - name: check-failures
        type: deterministic
        action: make_failed_executions
        vars:
          scenario_id: "12345"

      - name: diagnose
        type: ai
        skill: optimize-scenario

      - name: alert
        type: deterministic
        action: notify

  # Draft a LinkedIn post from a topic, human approves
  - name: content-draft
    schedule: manual
    steps:
      - name: mock-input
        type: deterministic

      - name: draft
        type: ai
        skill: draft-post

      - name: review
        type: approval
        mode: hitl
        channel: telegram

  # Weekly audit of n8n workflows
  - name: workflow-audit
    schedule: 7d
    steps:
      - name: fetch-workflows
        type: deterministic
        action: n8n_list_workflows

      - name: analyze
        type: ai
        skill: optimize-scenario

      - name: report
        type: deterministic
        action: notify
```

## Governance

Same guardrails as production operations tools:

- **Token budgets** -- per-step, per-pipeline, per-day limits with hard circuit breakers
- **Human-in-the-loop** -- every piece of outbound content requires operator approval
- **Output validation** -- AI output validated against JSON schemas
- **Audit trail** -- full logging of what was drafted, reviewed, and published

## Configuration

### config.yaml

```yaml
make:
  api_key_env: MAKE_API_KEY
  region: eu1
  team_id: 12345

n8n:
  base_url: https://your-n8n.example.com
  api_key_env: N8N_API_KEY

notion:
  api_key_env: NOTION_API_KEY

provider:
  type: openrouter
  api_key_env: OPENROUTER_API_KEY
  base_url: https://openrouter.ai/api/v1

budgets:
  per_step_tokens: 2048
  per_pipeline_tokens: 10000
  per_day_tokens: 100000
```

### Skills

YAML prompt templates in `skills/`. Each skill defines the system prompt, input variables, and output schema.

```yaml
# skills/draft-post.yaml
name: draft-post
role: drafter
prompt: |
  Topic: {{topic}}
  Platform: {{platform}}
  Write for practitioners, not beginners...
output_schema:
  title: {type: string}
  body: {type: string}
  platform: {type: string}
```

## Project Structure

```
redaktflow/
  main.go          # Engine: pipeline runner, scheduler, guardrails
  cli.go           # Agent-callable subcommand interface (JSON output)
  make.go          # Make.com integration (scenarios, executions, blueprints)
  n8n.go           # n8n integration (workflows, executions, credentials)
  notion.go        # Notion integration (databases, pages, content calendar)
  config.yaml      # Pipelines, models, budgets, timeouts
  secrets.yaml     # Private config (operator IDs) -- gitignored
  skills/          # Prompt templates with schema validation
    draft-post.yaml
    repurpose-content.yaml
    optimize-scenario.yaml
    schedule-content.yaml
```

## Star History

If RedaktFlow is useful to you, consider giving it a star. It helps others discover the project.

[![Star History Chart](https://api.star-history.com/svg?repos=renezander030/redaktflow&type=Date)](https://star-history.com/#renezander030/redaktflow&Date)

## License

MIT. See [LICENSE](LICENSE).
