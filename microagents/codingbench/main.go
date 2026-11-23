// Command codingbench runs the micro agents on small coding tasks that they
// solve with their file tools, through the memoizing model backend.
//
//	codingbench -mode record -run DIR [-scripted | -upstream URL]
//	codingbench -mode replay -run DIR
//
// Record runs the CX port of each agent in record mode on each task: the
// task's files/ become /spiffs, prompt.txt is the inbound message, and
// model requests go through backendstore serve -mode record, which forwards
// to -upstream (a real model, key in -api-key-env) or, with -scripted, to
// the stub with the task's solution.json as tool directives. check.sh then
// judges the files the agent left. DIR receives each tape and output, and
// results.md the table.
//
// Replay needs no model. It replays each tape with ++apply=1, which rebuilds
// the files the port wrote, and judges them again. It also runs the
// unmodified original agent on the task against a store filled from the
// tape, and judges its files too.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// agents are the agents with file tools. zclaw has none and is reported so.
var agents = []string{"femtoclaw", "mimiclaw"}

type config struct {
	root         string
	runDir       string
	cx           string
	backendstore string
	upstream     string
	apiKeyEnv    string
	scripted     bool
	tasks        []string
	agents       []string
}

type result struct {
	task, agent    string
	solved         string
	originalSolved string
	turns, calls   int
	sent, received int
	wall           time.Duration
	note           string
}

func main() {
	var c config
	mode := flag.String("mode", "replay", "record or replay")
	flag.StringVar(&c.runDir, "run", "", "run directory that holds the tapes")
	flag.StringVar(&c.root, "dir", ".", "the microagents folder")
	flag.StringVar(&c.cx, "cx", os.Getenv("CX"), "cx binary")
	flag.StringVar(&c.backendstore, "backendstore", os.Getenv("BACKENDSTORE"), "backendstore binary")
	flag.StringVar(&c.upstream, "upstream", "", "model API URL for record, e.g. https://api.anthropic.com/v1/messages")
	flag.StringVar(&c.apiKeyEnv, "api-key-env", "ANTHROPIC_API_KEY", "environment variable with the upstream API key")
	flag.BoolVar(&c.scripted, "scripted", false, "record against the stub with each task's solution.json")
	taskList := flag.String("tasks", "", "comma separated tasks, default all")
	agentList := flag.String("agents", strings.Join(agents, ","), "comma separated agents")
	flag.Parse()
	if *taskList != "" {
		c.tasks = strings.Split(*taskList, ",")
	}
	c.agents = strings.Split(*agentList, ",")
	var err error
	switch *mode {
	case "record":
		err = record(&c)
	case "replay":
		err = replay(&c)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codingbench:", err)
		os.Exit(1)
	}
}

func (c *config) check() error {
	if c.runDir == "" || c.cx == "" || c.backendstore == "" {
		return errors.New("-run, -cx and -backendstore are required")
	}
	var err error
	if c.root, err = filepath.Abs(c.root); err != nil {
		return err
	}
	if c.runDir, err = filepath.Abs(c.runDir); err != nil {
		return err
	}
	if c.tasks == nil {
		entries, err := os.ReadDir(filepath.Join(c.root, "codingbench", "tasks"))
		if err != nil {
			return err
		}
		for _, e := range entries {
			c.tasks = append(c.tasks, e.Name())
		}
	}
	return nil
}

func (c *config) taskDir(task string) string {
	return filepath.Join(c.root, "codingbench", "tasks", task)
}

func record(c *config) error {
	if err := c.check(); err != nil {
		return err
	}
	if !c.scripted && c.upstream == "" {
		return errors.New("record needs -upstream URL for a real model, or -scripted")
	}
	if !c.scripted && os.Getenv(c.apiKeyEnv) == "" {
		return fmt.Errorf("record against %s needs the API key in %s", c.upstream, c.apiKeyEnv)
	}
	var results []result
	for _, task := range c.tasks {
		for _, agent := range c.agents {
			r, err := recordOne(c, task, agent)
			if err != nil {
				return fmt.Errorf("%s %s: %w", task, agent, err)
			}
			results = append(results, r)
			fmt.Fprintf(os.Stderr, "%s %s: %s\n", task, agent, r.solved)
		}
	}
	model := "the scripted stub (solution.json of each task)"
	if !c.scripted {
		model = c.upstream
	}
	table := renderTable(results, false)
	text := fmt.Sprintf("# Coding benchmark run\n\nModel: %s\nRecorded: %s\n\n%s", model, time.Now().UTC().Format("2006-01-02 15:04 UTC"), table)
	if err := os.WriteFile(filepath.Join(c.runDir, "results.md"), []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Print(table)
	return nil
}

func recordOne(c *config, task, agent string) (result, error) {
	r := result{task: task, agent: agent}
	work, err := os.MkdirTemp("", "codingbench-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(work)
	if err := setupFiles(c.taskDir(task), filepath.Join(work, "spiffs")); err != nil {
		return r, err
	}
	message, err := taskMessage(c, task)
	if err != nil {
		return r, err
	}
	inbound := "telegram\t1\t" + escapeField(message) + "\n"
	if err := os.WriteFile(filepath.Join(work, "inbound.txt"), []byte(inbound), 0o644); err != nil {
		return r, err
	}

	upstream := c.upstream
	var stub *server
	if c.scripted {
		if stub, err = startServer(c.backendstore, work, "stub.url", "stub"); err != nil {
			return r, err
		}
		defer stub.stop()
		upstream = stub.url + "/v1/messages"
	}
	store, err := startServer(c.backendstore, work, "store.url", "serve", "-store", "store.bolt", "-mode", "record",
		"-listen", "127.0.0.1:18080", "-upstream", upstream, "-api-key-env", c.apiKeyEnv)
	if err != nil {
		return r, err
	}
	ports, err := filepath.Glob(filepath.Join(c.root, agent, "port", "*.cx"))
	if err != nil {
		return r, err
	}
	args := append(ports, "++mode=record", "++tape=tape.txt", "++inbound=inbound.txt",
		"++client="+c.backendstore, "++backend="+store.url)
	start := time.Now()
	output, runErr := runIn(work, c.cx, args...)
	r.wall = time.Since(start)
	store.stop()
	if runErr != nil {
		return r, fmt.Errorf("port: %v\n%s", runErr, output)
	}
	tape, err := os.ReadFile(filepath.Join(work, "tape.txt"))
	if err != nil {
		return r, err
	}
	r.turns, r.calls, r.sent, r.received = tapeMetrics(tape)
	r.solved, r.note = judge(c, task, filepath.Join(work, "spiffs"))
	saveDir := filepath.Join(c.runDir, agent, task)
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return r, err
	}
	if err := os.WriteFile(filepath.Join(saveDir, "tape.txt"), tape, 0o644); err != nil {
		return r, err
	}
	summary := fmt.Sprintf("solved=%s turns=%d calls=%d sent=%d received=%d wall_ms=%d\n",
		r.solved, r.turns, r.calls, r.sent, r.received, r.wall.Milliseconds())
	if err := os.WriteFile(filepath.Join(saveDir, "summary.txt"), []byte(summary), 0o644); err != nil {
		return r, err
	}
	return r, os.WriteFile(filepath.Join(saveDir, "output.txt"), output, 0o644)
}

func replay(c *config) error {
	if err := c.check(); err != nil {
		return err
	}
	var results []result
	failed := 0
	for _, task := range c.tasks {
		for _, agent := range c.agents {
			saveDir := filepath.Join(c.runDir, agent, task)
			if _, err := os.Stat(filepath.Join(saveDir, "tape.txt")); err != nil {
				continue
			}
			r, err := replayOne(c, task, agent, saveDir)
			if err != nil {
				failed++
				r.note = err.Error()
			}
			results = append(results, r)
		}
	}
	fmt.Print(renderTable(results, true))
	if failed > 0 {
		return fmt.Errorf("%d replay(s) diverged from the recording", failed)
	}
	return nil
}

func replayOne(c *config, task, agent, saveDir string) (result, error) {
	r := result{task: task, agent: agent, solved: "?", originalSolved: "?"}
	tape, err := os.ReadFile(filepath.Join(saveDir, "tape.txt"))
	if err != nil {
		return r, err
	}
	r.turns, r.calls, r.sent, r.received = tapeMetrics(tape)
	work, err := os.MkdirTemp("", "codingbench-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(work)
	if err := os.WriteFile(filepath.Join(work, "tape.txt"), tape, 0o644); err != nil {
		return r, err
	}
	if err := setupFiles(c.taskDir(task), filepath.Join(work, "spiffs")); err != nil {
		return r, err
	}
	ports, err := filepath.Glob(filepath.Join(c.root, agent, "port", "*.cx"))
	if err != nil {
		return r, err
	}
	start := time.Now()
	output, err := runIn(work, c.cx, append(ports, "++mode=replay", "++tape=tape.txt", "++apply=1")...)
	r.wall = time.Since(start)
	if err != nil {
		return r, fmt.Errorf("port replay: %v: %s", err, lastLine(output))
	}
	recorded, err := os.ReadFile(filepath.Join(saveDir, "output.txt"))
	if err != nil {
		return r, err
	}
	if !bytes.Equal(output, recorded) {
		return r, errors.New("the port's replay output differs from the recorded output")
	}
	r.solved, r.note = judge(c, task, filepath.Join(work, "spiffs"))

	before := filepath.Join(work, "before")
	if err := setupFiles(c.taskDir(task), filepath.Join(before, "spiffs")); err != nil {
		return r, err
	}
	if _, err := runIn(work, c.backendstore, "import", "-tape", "tape.txt", "-store", "store.bolt"); err != nil {
		return r, err
	}
	store, err := startServer(c.backendstore, work, "replay.url", "serve", "-store", "store.bolt", "-mode", "replay")
	if err != nil {
		return r, err
	}
	original := filepath.Join(c.root, "build", agent+"-before")
	if v := os.Getenv(strings.ToUpper(agent) + "_BEFORE"); v != "" {
		original = v
	}
	beforeOutput, beforeErr := runIn(before, original, "../tape.txt", store.url, c.backendstore)
	if err := store.stop(); err != nil {
		return r, fmt.Errorf("the store missed requests of the original: %v", err)
	}
	if beforeErr != nil {
		return r, fmt.Errorf("original: %v: %s", beforeErr, lastLine(beforeOutput))
	}
	if !bytes.Equal(beforeOutput, output) {
		return r, errors.New("the original printed other bytes than the port")
	}
	r.originalSolved, _ = judge(c, task, filepath.Join(before, "spiffs"))
	return r, nil
}

// setupFiles copies the task's files/ to dst, plus the sessions folder the
// agents append chat history to.
func setupFiles(taskDir, dst string) error {
	src := filepath.Join(taskDir, "files")
	if err := os.MkdirAll(filepath.Join(dst, "sessions"), 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

type toolCall struct {
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// taskMessage is the inbound message of a task: prompt.txt, and with
// -scripted the steps of solution.json as stub directives after it.
func taskMessage(c *config, task string) (string, error) {
	prompt, err := os.ReadFile(filepath.Join(c.taskDir(task), "prompt.txt"))
	if err != nil {
		return "", err
	}
	message := strings.TrimSpace(string(prompt))
	if !c.scripted {
		return message, nil
	}
	data, err := os.ReadFile(filepath.Join(c.taskDir(task), "solution.json"))
	if err != nil {
		return "", err
	}
	var steps [][]toolCall
	if err := json.Unmarshal(data, &steps); err != nil {
		return "", fmt.Errorf("solution.json: %w", err)
	}
	var parts []string
	for _, step := range steps {
		var calls []string
		for _, call := range step {
			var input bytes.Buffer
			if err := json.Compact(&input, call.Input); err != nil {
				return "", err
			}
			if strings.Contains(input.String(), "]]") {
				return "", fmt.Errorf("solution.json: %s input holds ]], which ends a stub directive", call.Tool)
			}
			calls = append(calls, fmt.Sprintf("[[tool %s %s]]", call.Tool, input.String()))
		}
		parts = append(parts, strings.Join(calls, " "))
	}
	return message + " " + strings.Join(parts, " [[then]] "), nil
}

// escapeField escapes a line of inbound.txt.
func escapeField(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
}

func unescapeField(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// tapeMetrics counts the model turns, the tool calls the model asked for,
// and the request and reply bytes on a tape.
func tapeMetrics(tape []byte) (turns, calls, sent, received int) {
	for _, line := range strings.Split(string(tape), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		switch fields[0] {
		case "net-request":
			turns++
			sent += len(unescapeField(fields[2]))
		case "net-reply":
			body := unescapeField(fields[2])
			received += len(body)
			var reply struct {
				Content []struct {
					Type string `json:"type"`
				} `json:"content"`
			}
			if json.Unmarshal([]byte(body), &reply) == nil {
				for _, block := range reply.Content {
					if block.Type == "tool_use" {
						calls++
					}
				}
			}
		}
	}
	return
}

func judge(c *config, task, tree string) (string, string) {
	cmd := exec.Command("/bin/sh", filepath.Join(c.taskDir(task), "check.sh"))
	cmd.Dir = tree
	cmd.Env = append(os.Environ(), "CX="+c.cx)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "no", lastLine(out)
	}
	return "yes", ""
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

func runIn(dir, name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		stdout.Write(stderr.Bytes())
	}
	return stdout.Bytes(), err
}

type server struct {
	cmd *exec.Cmd
	url string
	bin string
}

// startServer starts backendstore with args and waits for the URL it
// writes to addressFile in dir.
func startServer(bin, dir, addressFile string, args ...string) (*server, error) {
	cmd := exec.Command(bin, append(args, "-address-file", addressFile)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(dir, addressFile)); err == nil && len(data) > 0 {
			return &server{cmd: cmd, url: strings.TrimSpace(string(data)), bin: bin}, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	cmd.Process.Kill()
	return nil, fmt.Errorf("%s %s did not start", bin, args[0])
}

func (s *server) stop() error {
	if s.cmd.ProcessState != nil {
		return nil
	}
	if err := exec.Command(s.bin, "stop", "-url", s.url).Run(); err != nil {
		return err
	}
	return s.cmd.Wait()
}

func renderTable(results []result, replay bool) string {
	sort.SliceStable(results, func(i, j int) bool { return results[i].task < results[j].task })
	var b strings.Builder
	if replay {
		b.WriteString("| Task | Agent | Port solved | Original solved | Model turns | Tool calls | Bytes sent | Bytes received | Replay wall time | Note |\n|---|---|---|---|---|---|---|---|---|---|\n")
	} else {
		b.WriteString("| Task | Agent | Solved | Model turns | Tool calls | Bytes sent | Bytes received | Wall time | Note |\n|---|---|---|---|---|---|---|---|---|\n")
	}
	solved := map[string]int{}
	for _, r := range results {
		wall := fmt.Sprintf("%.1f s", r.wall.Seconds())
		if replay {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %d | %d | %d | %s | %s |\n", r.task, r.agent, r.solved, r.originalSolved, r.turns, r.calls, r.sent, r.received, wall, r.note)
		} else {
			fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %d | %s | %s |\n", r.task, r.agent, r.solved, r.turns, r.calls, r.sent, r.received, wall, r.note)
		}
		if r.solved == "yes" {
			solved[r.agent]++
		}
	}
	tasks := map[string]bool{}
	for _, r := range results {
		tasks[r.task] = true
	}
	var names []string
	for agent := range solved {
		names = append(names, agent)
	}
	sort.Strings(names)
	b.WriteString("\n")
	for _, agent := range names {
		fmt.Fprintf(&b, "%s solved %d of %d tasks. ", agent, solved[agent], len(tasks))
	}
	b.WriteString("zclaw is not run: it has no file tools.\n")
	return b.String()
}
