package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func stubCalls(t *testing.T, body string) (string, []stubContentBlock) {
	t.Helper()
	status, data, err := StubReplyFor([]byte(body))
	if err != nil || status != 200 {
		t.Fatalf("status %d, err %v", status, err)
	}
	var reply stubReply
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatal(err)
	}
	var calls []stubContentBlock
	for _, block := range reply.Content {
		if block.Type == "tool_use" {
			calls = append(calls, block)
		}
	}
	return reply.StopReason, calls
}

func TestStubSteps(t *testing.T) {
	prompt := `{"role":"user","content":"Copy [[tool read_file {\"path\":\"/spiffs/a\"}]] [[then]] [[tool write_file {\"path\":\"/spiffs/b\",\"content\":\"{{result 1}}\"}]]"}`
	stop, calls := stubCalls(t, `{"model":"m","messages":[`+prompt+`]}`)
	if stop != "tool_use" || len(calls) != 1 || calls[0].Name != "read_file" {
		t.Fatalf("step 1: %s %+v", stop, calls)
	}
	afterRead := `{"model":"m","messages":[` + prompt + `,{"role":"assistant","content":[]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"line \"one\"\n"}]}]}`
	stop, calls = stubCalls(t, afterRead)
	if stop != "tool_use" || len(calls) != 1 || calls[0].Name != "write_file" {
		t.Fatalf("step 2: %s %+v", stop, calls)
	}
	var input map[string]string
	if err := json.Unmarshal(calls[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["content"] != "line \"one\"\n" {
		t.Fatalf("step 2 content %q", input["content"])
	}
	afterWrite := afterRead[:len(afterRead)-2] + `,{"role":"assistant","content":[]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"y","content":"OK"}]}]}`
	if stop, calls = stubCalls(t, afterWrite); stop != "end_turn" || len(calls) != 0 {
		t.Fatalf("after the last step: %s %+v", stop, calls)
	}
}

func TestListFilesIsFlatAndSorted(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b.md", "memory/a.md", "a.md", "skills/z/x.md"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := listFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.md", "b.md", "memory/a.md", "skills/z/x.md"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("got %q, want %q", names, want)
	}
}

func TestImportFillsStoreFromTape(t *testing.T) {
	dir := t.TempDir()
	tape := "clock\t1\nnet-request\thttp://x/v1/messages\t{\"a\":\"\\\\n\"}\nnet-reply\t200\t{\"ok\":1}\\n\ninbound-end\n"
	tapePath := filepath.Join(dir, "tape.txt")
	storePath := filepath.Join(dir, "store.bolt")
	if err := os.WriteFile(tapePath, []byte(tape), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runImport([]string{"-tape", tapePath, "-store", storePath}); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(storePath, 0o644, &bolt.Options{Timeout: time.Second, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &memoServer{db: db}
	status, response, found, err := s.lookup(RequestKey([]byte(`{"a":"\n"}`)))
	if err != nil || !found || status != 200 || string(response) != "{\"ok\":1}\n" {
		t.Fatalf("lookup: %d %q %v %v", status, response, found, err)
	}
}
