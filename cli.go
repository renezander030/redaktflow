package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// --- CLI Subcommand Router ---
// All commands output JSON by default. Agent-first interface.

type CLIResult struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Count   int         `json:"count,omitempty"`
}

func printResult(result CLIResult) {
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func cliError(cmd string, err error) {
	printResult(CLIResult{OK: false, Command: cmd, Error: err.Error()})
	os.Exit(1)
}

// runCLI routes subcommands. Returns true if a subcommand was handled.
func runCLI(args []string, cfg Config) bool {
	if len(args) < 2 {
		return false
	}

	group := args[1]

	switch group {
	case "notion":
		return runNotionCLI(args[2:], cfg)
	case "make":
		return runMakeCLI(args[2:], cfg)
	case "n8n":
		return runN8NCLI(args[2:], cfg)
	case "status":
		return runStatusCLI(cfg)
	case "help":
		printHelp()
		return true
	default:
		return false
	}
}

func printHelp() {
	help := CLIResult{
		OK:      true,
		Command: "help",
		Data: map[string]interface{}{
			"usage": "redaktflow <group> <command> [args]",
			"groups": map[string]interface{}{
				"notion": []string{
					"list-dbs",
					"get-db --id <database_id>",
					"query --db <database_id> [--filter '<json>'] [--limit N]",
					"create-db --parent <page_id> --name <name> --props '<json>'",
					"create-page --db <database_id> --props '<json>'",
					"update-page --id <page_id> --props '<json>'",
				},
				"make": []string{
					"list-scenarios",
					"get-scenario --id <scenario_id>",
					"get-blueprint --id <scenario_id>",
					"set-blueprint --id <scenario_id> --file <blueprint.json>",
					"create-scenario --name <name> --blueprint <file.json>",
					"run --id <scenario_id>",
					"executions --id <scenario_id> [--status error] [--limit N]",
					"activate --id <scenario_id>",
					"deactivate --id <scenario_id>",
				},
				"n8n": []string{
					"list-workflows",
					"get-workflow --id <workflow_id>",
					"create-workflow --name <name> --file <workflow.json>",
					"executions [--id <workflow_id>] [--status error] [--limit N]",
					"retry --id <execution_id>",
					"activate --id <workflow_id>",
					"deactivate --id <workflow_id>",
				},
				"status": "Show connection status for all configured platforms",
			},
			"modes": map[string]string{
				"daemon":   "redaktflow (no args) — run scheduled pipelines",
				"one-shot": "redaktflow --run <pipeline> — run single pipeline and exit",
				"cli":      "redaktflow <group> <command> — agent-callable subcommands",
			},
		},
	}
	printResult(help)
}

// --- Notion CLI ---

func runNotionCLI(args []string, cfg Config) bool {
	if len(args) == 0 {
		cliError("notion", fmt.Errorf("missing command. Use: list-dbs, get-db, query, create-db, create-page, update-page"))
		return true
	}

	if notionConn == nil {
		cliError("notion "+args[0], fmt.Errorf("notion connector not configured — set %s env var", cfg.Notion.APIKeyEnv))
		return true
	}

	cmd := args[0]
	flags := parseFlags(args[1:])

	switch cmd {
	case "list-dbs":
		dbs, err := notionConn.ListDatabases()
		if err != nil {
			cliError("notion list-dbs", err)
		}
		printResult(CLIResult{OK: true, Command: "notion list-dbs", Data: dbs, Count: len(dbs)})

	case "get-db":
		id := flags["id"]
		if id == "" {
			cliError("notion get-db", fmt.Errorf("--id required"))
		}
		db, err := notionConn.GetDatabase(id)
		if err != nil {
			cliError("notion get-db", err)
		}
		printResult(CLIResult{OK: true, Command: "notion get-db", Data: db})

	case "query":
		dbID := flags["db"]
		if dbID == "" {
			cliError("notion query", fmt.Errorf("--db required"))
		}
		limit := 20
		if l, ok := flags["limit"]; ok {
			limit, _ = strconv.Atoi(l)
		}
		var filter json.RawMessage
		if f, ok := flags["filter"]; ok && f != "" {
			filter = json.RawMessage(f)
		}
		pages, err := notionConn.QueryDatabase(dbID, filter, limit)
		if err != nil {
			cliError("notion query", err)
		}
		printResult(CLIResult{OK: true, Command: "notion query", Data: pages, Count: len(pages)})

	case "create-db":
		parentID := flags["parent"]
		name := flags["name"]
		propsJSON := flags["props"]
		if parentID == "" || name == "" {
			cliError("notion create-db", fmt.Errorf("--parent and --name required"))
		}

		properties := map[string]interface{}{
			"Name": map[string]interface{}{"title": map[string]interface{}{}},
		}

		// Parse additional properties
		if propsJSON != "" {
			var extraProps map[string]string
			if err := json.Unmarshal([]byte(propsJSON), &extraProps); err != nil {
				cliError("notion create-db", fmt.Errorf("--props must be valid JSON: %w", err))
			}
			for propName, propType := range extraProps {
				if propName == "Name" {
					continue
				}
				properties[propName] = map[string]interface{}{propType: map[string]interface{}{}}
			}
		}

		body := map[string]interface{}{
			"parent": map[string]string{"type": "page_id", "page_id": parentID},
			"title":  []map[string]interface{}{{"type": "text", "text": map[string]string{"content": name}}},
			"properties": properties,
		}

		data, err := notionConn.request("POST", "/databases", body)
		if err != nil {
			cliError("notion create-db", err)
		}
		var db NotionDatabase
		json.Unmarshal(data, &db)
		printResult(CLIResult{OK: true, Command: "notion create-db", Data: db})

	case "create-page":
		dbID := flags["db"]
		propsJSON := flags["props"]
		if dbID == "" || propsJSON == "" {
			cliError("notion create-page", fmt.Errorf("--db and --props required"))
		}
		var props map[string]interface{}
		if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
			cliError("notion create-page", fmt.Errorf("--props must be valid JSON: %w", err))
		}
		page, err := notionConn.CreatePage(dbID, props)
		if err != nil {
			cliError("notion create-page", err)
		}
		printResult(CLIResult{OK: true, Command: "notion create-page", Data: page})

	case "update-page":
		pageID := flags["id"]
		propsJSON := flags["props"]
		if pageID == "" || propsJSON == "" {
			cliError("notion update-page", fmt.Errorf("--id and --props required"))
		}
		var props map[string]interface{}
		if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
			cliError("notion update-page", fmt.Errorf("--props must be valid JSON: %w", err))
		}
		if err := notionConn.UpdatePage(pageID, props); err != nil {
			cliError("notion update-page", err)
		}
		printResult(CLIResult{OK: true, Command: "notion update-page", Data: map[string]string{"id": pageID, "status": "updated"}})

	default:
		cliError("notion "+cmd, fmt.Errorf("unknown command. Use: list-dbs, get-db, query, create-db, create-page, update-page"))
	}

	return true
}

// --- Make.com CLI ---

func runMakeCLI(args []string, cfg Config) bool {
	if len(args) == 0 {
		cliError("make", fmt.Errorf("missing command. Use: list-scenarios, get-scenario, get-blueprint, set-blueprint, create-scenario, run, executions, activate, deactivate"))
		return true
	}

	if makeConn == nil {
		cliError("make "+args[0], fmt.Errorf("make connector not configured — set %s env var", cfg.Make.APIKeyEnv))
		return true
	}

	cmd := args[0]
	flags := parseFlags(args[1:])

	switch cmd {
	case "list-scenarios":
		scenarios, err := makeConn.ListScenarios()
		if err != nil {
			cliError("make list-scenarios", err)
		}
		printResult(CLIResult{OK: true, Command: "make list-scenarios", Data: scenarios, Count: len(scenarios)})

	case "get-scenario":
		id, err := requireIntFlag(flags, "id", "make get-scenario")
		if err != nil {
			cliError("make get-scenario", err)
		}
		scenario, err := makeConn.GetScenario(id)
		if err != nil {
			cliError("make get-scenario", err)
		}
		printResult(CLIResult{OK: true, Command: "make get-scenario", Data: scenario})

	case "get-blueprint":
		id, err := requireIntFlag(flags, "id", "make get-blueprint")
		if err != nil {
			cliError("make get-blueprint", err)
		}
		bp, err := makeConn.GetBlueprint(id)
		if err != nil {
			cliError("make get-blueprint", err)
		}
		printResult(CLIResult{OK: true, Command: "make get-blueprint", Data: bp})

	case "set-blueprint":
		id, err := requireIntFlag(flags, "id", "make set-blueprint")
		if err != nil {
			cliError("make set-blueprint", err)
		}
		filePath := flags["file"]
		if filePath == "" {
			cliError("make set-blueprint", fmt.Errorf("--file required"))
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			cliError("make set-blueprint", fmt.Errorf("failed to read blueprint file: %w", err))
		}
		if err := makeConn.SetBlueprint(id, json.RawMessage(data)); err != nil {
			cliError("make set-blueprint", err)
		}
		printResult(CLIResult{OK: true, Command: "make set-blueprint", Data: map[string]interface{}{"scenario_id": id, "status": "updated"}})

	case "create-scenario":
		name := flags["name"]
		if name == "" {
			cliError("make create-scenario", fmt.Errorf("--name required"))
		}
		var blueprint json.RawMessage
		if filePath, ok := flags["blueprint"]; ok && filePath != "" {
			data, err := os.ReadFile(filePath)
			if err != nil {
				cliError("make create-scenario", fmt.Errorf("failed to read blueprint file: %w", err))
			}
			blueprint = data
		}
		scenario, err := makeConn.CreateScenario(name, cfg.Make.TeamID, blueprint)
		if err != nil {
			cliError("make create-scenario", err)
		}
		printResult(CLIResult{OK: true, Command: "make create-scenario", Data: scenario})

	case "run":
		id, err := requireIntFlag(flags, "id", "make run")
		if err != nil {
			cliError("make run", err)
		}
		if err := makeConn.RunScenario(id); err != nil {
			cliError("make run", err)
		}
		printResult(CLIResult{OK: true, Command: "make run", Data: map[string]interface{}{"scenario_id": id, "status": "triggered"}})

	case "executions":
		id, err := requireIntFlag(flags, "id", "make executions")
		if err != nil {
			cliError("make executions", err)
		}
		limit := 10
		if l, ok := flags["limit"]; ok {
			limit, _ = strconv.Atoi(l)
		}
		status := flags["status"]

		var execs []MakeExecution
		if status == "error" {
			execs, err = makeConn.ListFailedExecutions(id, limit)
		} else {
			execs, err = makeConn.ListExecutions(id, limit)
		}
		if err != nil {
			cliError("make executions", err)
		}
		printResult(CLIResult{OK: true, Command: "make executions", Data: execs, Count: len(execs)})

	case "activate":
		id, err := requireIntFlag(flags, "id", "make activate")
		if err != nil {
			cliError("make activate", err)
		}
		if err := makeConn.ActivateScenario(id); err != nil {
			cliError("make activate", err)
		}
		printResult(CLIResult{OK: true, Command: "make activate", Data: map[string]interface{}{"scenario_id": id, "status": "activated"}})

	case "deactivate":
		id, err := requireIntFlag(flags, "id", "make deactivate")
		if err != nil {
			cliError("make deactivate", err)
		}
		if err := makeConn.DeactivateScenario(id); err != nil {
			cliError("make deactivate", err)
		}
		printResult(CLIResult{OK: true, Command: "make deactivate", Data: map[string]interface{}{"scenario_id": id, "status": "deactivated"}})

	default:
		cliError("make "+cmd, fmt.Errorf("unknown command. Use: list-scenarios, get-scenario, get-blueprint, set-blueprint, create-scenario, run, executions, activate, deactivate"))
	}

	return true
}

// --- n8n CLI ---

func runN8NCLI(args []string, cfg Config) bool {
	if len(args) == 0 {
		cliError("n8n", fmt.Errorf("missing command. Use: list-workflows, get-workflow, create-workflow, executions, retry, activate, deactivate"))
		return true
	}

	if n8nConn == nil {
		cliError("n8n "+args[0], fmt.Errorf("n8n connector not configured — set %s env var and base_url", cfg.N8N.APIKeyEnv))
		return true
	}

	cmd := args[0]
	flags := parseFlags(args[1:])

	switch cmd {
	case "list-workflows":
		workflows, err := n8nConn.ListWorkflows()
		if err != nil {
			cliError("n8n list-workflows", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n list-workflows", Data: workflows, Count: len(workflows)})

	case "get-workflow":
		id := flags["id"]
		if id == "" {
			cliError("n8n get-workflow", fmt.Errorf("--id required"))
		}
		wf, err := n8nConn.GetWorkflow(id)
		if err != nil {
			cliError("n8n get-workflow", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n get-workflow", Data: wf})

	case "create-workflow":
		name := flags["name"]
		if name == "" {
			cliError("n8n create-workflow", fmt.Errorf("--name required"))
		}
		var nodes []N8NNode
		if filePath, ok := flags["file"]; ok && filePath != "" {
			data, err := os.ReadFile(filePath)
			if err != nil {
				cliError("n8n create-workflow", fmt.Errorf("failed to read workflow file: %w", err))
			}
			if err := json.Unmarshal(data, &nodes); err != nil {
				cliError("n8n create-workflow", fmt.Errorf("invalid workflow JSON: %w", err))
			}
		}
		wf, err := n8nConn.CreateWorkflow(name, nodes)
		if err != nil {
			cliError("n8n create-workflow", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n create-workflow", Data: wf})

	case "executions":
		workflowID := flags["id"]
		limit := 10
		if l, ok := flags["limit"]; ok {
			limit, _ = strconv.Atoi(l)
		}
		status := flags["status"]

		var execs []N8NExecution
		var err error
		if status == "error" {
			execs, err = n8nConn.ListFailedExecutions(workflowID, limit)
		} else {
			execs, err = n8nConn.ListExecutions(workflowID, limit)
		}
		if err != nil {
			cliError("n8n executions", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n executions", Data: execs, Count: len(execs)})

	case "retry":
		id := flags["id"]
		if id == "" {
			cliError("n8n retry", fmt.Errorf("--id required"))
		}
		if err := n8nConn.RetryExecution(id); err != nil {
			cliError("n8n retry", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n retry", Data: map[string]string{"execution_id": id, "status": "retried"}})

	case "activate":
		id := flags["id"]
		if id == "" {
			cliError("n8n activate", fmt.Errorf("--id required"))
		}
		if err := n8nConn.ActivateWorkflow(id); err != nil {
			cliError("n8n activate", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n activate", Data: map[string]string{"workflow_id": id, "status": "activated"}})

	case "deactivate":
		id := flags["id"]
		if id == "" {
			cliError("n8n deactivate", fmt.Errorf("--id required"))
		}
		if err := n8nConn.DeactivateWorkflow(id); err != nil {
			cliError("n8n deactivate", err)
		}
		printResult(CLIResult{OK: true, Command: "n8n deactivate", Data: map[string]string{"workflow_id": id, "status": "deactivated"}})

	default:
		cliError("n8n "+cmd, fmt.Errorf("unknown command. Use: list-workflows, get-workflow, create-workflow, executions, retry, activate, deactivate"))
	}

	return true
}

// --- Status ---

func runStatusCLI(cfg Config) bool {
	status := map[string]interface{}{}

	// Make.com
	if makeConn != nil {
		scenarios, err := makeConn.ListScenarios()
		if err != nil {
			status["make"] = map[string]interface{}{"connected": false, "error": err.Error()}
		} else {
			active := 0
			for _, s := range scenarios {
				if s.IsEnabled {
					active++
				}
			}
			status["make"] = map[string]interface{}{
				"connected":      true,
				"region":         cfg.Make.Region,
				"team_id":        cfg.Make.TeamID,
				"total_scenarios": len(scenarios),
				"active":         active,
			}
		}
	} else {
		status["make"] = map[string]interface{}{"connected": false, "reason": "not configured"}
	}

	// n8n
	if n8nConn != nil {
		workflows, err := n8nConn.ListWorkflows()
		if err != nil {
			status["n8n"] = map[string]interface{}{"connected": false, "error": err.Error()}
		} else {
			active := 0
			for _, w := range workflows {
				if w.Active {
					active++
				}
			}
			status["n8n"] = map[string]interface{}{
				"connected":       true,
				"base_url":        cfg.N8N.BaseURL,
				"total_workflows": len(workflows),
				"active":          active,
			}
		}
	} else {
		status["n8n"] = map[string]interface{}{"connected": false, "reason": "not configured"}
	}

	// Notion
	if notionConn != nil {
		dbs, err := notionConn.ListDatabases()
		if err != nil {
			status["notion"] = map[string]interface{}{"connected": false, "error": err.Error()}
		} else {
			status["notion"] = map[string]interface{}{
				"connected":       true,
				"total_databases": len(dbs),
			}
		}
	} else {
		status["notion"] = map[string]interface{}{"connected": false, "reason": "not configured"}
	}

	printResult(CLIResult{OK: true, Command: "status", Data: status})
	return true
}

// --- Flag Parsing ---

func parseFlags(args []string) map[string]string {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				flags[key] = "true"
			}
		}
	}
	return flags
}

func requireIntFlag(flags map[string]string, name string, cmd string) (int, error) {
	val, ok := flags[name]
	if !ok || val == "" {
		return 0, fmt.Errorf("--%s required", name)
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("--%s must be a number: %s", name, val)
	}
	return n, nil
}
