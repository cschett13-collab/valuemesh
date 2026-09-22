// valuemesh — value-maximization as a system-wide protocol for an agent fleet.
//
// valuemax (its ancestor) only WATCHED. valuemesh makes the fleet accountable
// TOGETHER: every agent reports the OUTCOME it produced (not tokens burned), the
// hub reflects on each agent's real logs to grade value vs waste, and it writes a
// single shared DIRECTIVE that every agent reads — so improving the directive
// changes every agent at once. One fleet, measured together, improving together.
//
//   valuemesh report --agent NAME --did "what you accomplished" [--cost-usd N] [--saved-min M]
//   valuemesh ask NAME [--log PATH]     reflect on an agent's real logs (local model), grade value/waste
//   valuemesh board [--json]            the collective value view + (re)write the shared directive
//   valuemesh directive                 print the shared value-maxxing contract every agent follows
//
// Shared state lives on the mesh: ~/.valuemesh/{ledger.jsonl, directive.md}.
// Custom to Cole's stack: litellm :4000, modelgate :4060, his agent logs. Go stdlib.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var (
	home, _  = os.UserHomeDir()
	meshDir  = env("VALUEMESH_DIR", filepath.Join(home, ".valuemesh"))
	ledger   = filepath.Join(meshDir, "ledger.jsonl")
	directiv = filepath.Join(meshDir, "directive.md")
	base     = env("LITELLM_BASE", "http://127.0.0.1:4000")
	key      = env("LITELLM_KEY", "")
	model    = env("VALUEMESH_MODEL", "fast")
	mgStats  = env("MODELGATE_STATS", "http://127.0.0.1:4060/api/stats")
	logDir   = env("AGENT_LOG_DIR", filepath.Join(home, "Desktop/ai-agents/logs"))
	staleMin = 120
)

const defaultDirective = `# valuemesh directive — the shared value-maximization contract every agent follows
_Edit this one file to change every agent at once. Systems over models; outcomes over consumption._

1. **Report outcomes, not activity.** After real work, run:
   ` + "`valuemesh report --agent <you> --did \"<what you accomplished>\" [--saved-min N]`" + `
2. **Idle is waste.** If you are consuming with no outcome, stop and say so — don't burn the fleet's budget.
3. **Share what works.** A high-value pattern goes in the ledger so other agents can reuse it.
4. **One fleet, measured together.** Your value is judged against the whole, not in isolation.
`

type event struct {
	TS      string  `json:"ts"`
	Agent   string  `json:"agent"`
	Kind    string  `json:"kind"` // report | reflection
	Did     string  `json:"did"`
	CostUSD float64 `json:"cost_usd,omitempty"`
	SaveMin int     `json:"saved_min,omitempty"`
}

func appendEvent(e event) {
	os.MkdirAll(meshDir, 0o755)
	f, _ := os.OpenFile(ledger, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if f != nil {
		defer f.Close()
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
	}
}

func readLedger() []event {
	f, err := os.Open(ledger)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []event
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		var e event
		if json.Unmarshal(s.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func chat(system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model, "temperature": 0.2, "max_tokens": 260,
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
	})
	req, _ := http.NewRequest("POST", base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.Choices) == 0 {
		return "", fmt.Errorf("no reply")
	}
	c := r.Choices[0].Message.Content
	if i := strings.LastIndex(c, "</think>"); i >= 0 {
		c = c[i+len("</think>"):]
	}
	return strings.TrimSpace(c), nil
}

func tailFile(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func fleetCost() (float64, float64) { // revenue, spent
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(mgStats)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	var s struct {
		Revenue float64 `json:"revenue_onchain_usd"`
		Spent   float64 `json:"spent_usd"`
	}
	json.NewDecoder(resp.Body).Decode(&s)
	return s.Revenue, s.Spent
}

func staleAgents() []string {
	var out []string
	files, _ := filepath.Glob(filepath.Join(logDir, "*.status.json"))
	now := time.Now()
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		age := int(now.Sub(fi.ModTime()).Minutes())
		if age > staleMin {
			n := filepath.Base(f)
			out = append(out, fmt.Sprintf("%s (%dm idle)", n[:len(n)-len(".status.json")], age))
		}
	}
	return out
}

func ensureDirective() {
	if _, err := os.Stat(directiv); err != nil {
		os.MkdirAll(meshDir, 0o755)
		os.WriteFile(directiv, []byte(defaultDirective), 0o644)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: valuemesh report|ask|board|directive  (see -h on each)")
		return
	}
	switch os.Args[1] {
	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		agent := fs.String("agent", "", "agent name")
		did := fs.String("did", "", "what you accomplished (the outcome)")
		cost := fs.Float64("cost-usd", 0, "USD consumed")
		saved := fs.Int("saved-min", 0, "minutes of human time saved")
		fs.Parse(os.Args[2:])
		if *agent == "" || *did == "" {
			fmt.Fprintln(os.Stderr, "need --agent and --did")
			os.Exit(1)
		}
		appendEvent(event{time.Now().UTC().Format(time.RFC3339), *agent, "report", *did, *cost, *saved})
		fmt.Printf("logged outcome for %s ✓\n", *agent)

	case "ask":
		fs := flag.NewFlagSet("ask", flag.ExitOnError)
		logPath := fs.String("log", "", "path to the agent's log (default: <logdir>/<name>.log)")
		fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: valuemesh ask <agent> [--log PATH]")
			os.Exit(1)
		}
		name := fs.Arg(0)
		lp := *logPath
		if lp == "" {
			lp = filepath.Join(logDir, name+".log")
		}
		tail := tailFile(lp, 60)
		if strings.TrimSpace(tail) == "" {
			fmt.Printf("no readable log for %s at %s — cannot assess honestly (not fabricating a grade).\n", name, lp)
			return
		}
		sys := "You grade an AI agent on VALUE, not activity. From its real log, answer in 3 short lines: " +
			"(1) OUTCOME it actually produced; (2) WASTE/idle you see; (3) one concrete change to raise its value. Be blunt, no filler."
		out, err := chat(sys, "Agent: "+name+"\n\nRecent log:\n"+tail)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ask failed:", err)
			os.Exit(1)
		}
		appendEvent(event{time.Now().UTC().Format(time.RFC3339), name, "reflection", out, 0, 0})
		fmt.Printf("── value reflection: %s ──\n%s\n\n(logged to the shared ledger)\n", name, out)

	case "board":
		asJSON := len(os.Args) > 2 && os.Args[2] == "--json"
		ensureDirective()
		evs := readLedger()
		latest := map[string]event{}
		outcomes := map[string]int{}
		for _, e := range evs {
			outcomes[e.Agent]++
			latest[e.Agent] = e
		}
		rev, spent := fleetCost()
		stale := staleAgents()

		// rewrite the shared directive with the live waste callouts appended:
		// editing this file changes what every agent reads = "changes agents everywhere".
		body := defaultDirective + "\n---\n## Live fleet signals (auto, " + time.Now().Format("2006-01-02 15:04") + ")\n" +
			fmt.Sprintf("- fleet outcome vs cost: $%.4f revenue on $%.2f spent\n", rev, spent)
		if len(stale) > 0 {
			body += "- **fix now (idle/waste):** " + strings.Join(stale, ", ") + "\n"
		} else {
			body += "- no idle agents detected\n"
		}
		os.WriteFile(directiv, []byte(body), 0o644)

		if asJSON {
			agents := []map[string]any{}
			for a, n := range outcomes {
				agents = append(agents, map[string]any{"agent": a, "reports": n, "latest": latest[a].Did})
			}
			json.NewEncoder(os.Stdout).Encode(map[string]any{
				"revenue_usd": rev, "spent_usd": spent, "stale": stale, "agents": agents, "directive": directiv})
			return
		}
		fmt.Println("═══ valuemesh board — one fleet, measured together ═══")
		fmt.Printf("fleet: $%.4f revenue on $%.2f spent\n\n", rev, spent)
		if len(outcomes) == 0 {
			fmt.Println("no agents have reported outcomes yet — adopt the beacon:")
			fmt.Println("  valuemesh report --agent <name> --did \"<outcome>\"")
		} else {
			type row struct {
				a string
				n int
			}
			rows := []row{}
			for a, n := range outcomes {
				rows = append(rows, row{a, n})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
			fmt.Println("agents reporting value:")
			for _, r := range rows {
				fmt.Printf("  • %-16s %d report(s) — last: %s\n", r.a, r.n, trunc(latest[r.a].Did, 60))
			}
		}
		fmt.Printf("\nBAD STUFF (%d):\n", len(stale))
		for _, s := range stale {
			fmt.Println("  ⚠", s)
		}
		if len(stale) == 0 {
			fmt.Println("  ✓ none idle")
		}
		fmt.Println("\nshared directive rewritten →", directiv, "(every agent that reads it just changed)")

	case "directive":
		ensureDirective()
		b, _ := os.ReadFile(directiv)
		fmt.Print(string(b))

	default:
		fmt.Println("unknown command:", os.Args[1])
	}
}

func trunc(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}
