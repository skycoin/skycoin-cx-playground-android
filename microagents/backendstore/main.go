// Command backendstore is the memoizing model backend of the micro agent
// replay rig. It stores each model response under the SHA-256 of the exact
// request bytes in a bbolt file.
//
//	backendstore serve -store F -mode record|replay [-upstream URL]
//	backendstore stub
//	backendstore post -url URL -body FILE
//	backendstore list -store F
//	backendstore stop -url URL
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	responsesBucket = []byte("responses")
	statusesBucket  = []byte("statuses")
	requestsBucket  = []byte("requests")
)

const stopPath = "/backendstore/stop"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "stub":
		err = runStub(os.Args[2:])
	case "post":
		err = runPost(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	case "stop":
		err = runStop(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "backendstore:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: backendstore serve|stub|post|list|stop [flags]")
	os.Exit(2)
}

// RequestKey is the store key of a request, the hex SHA-256 of its bytes.
func RequestKey(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type memoServer struct {
	db          *bolt.DB
	replay      bool
	upstream    string
	apiKey      string
	apiVersion  string
	missMu      sync.Mutex
	missedKeys  []string
	stopChannel chan struct{}
}

func (s *memoServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == stopPath {
		w.WriteHeader(http.StatusOK)
		close(s.stopChannel)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key := RequestKey(body)
	status, response, found, err := s.lookup(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		if s.replay {
			s.missMu.Lock()
			s.missedKeys = append(s.missedKeys, key)
			s.missMu.Unlock()
			fmt.Fprintf(os.Stderr, "backendstore: replay miss %s (%d request bytes)\n", key, len(body))
			http.Error(w, "backendstore: replay miss "+key, http.StatusBadGateway)
			return
		}
		status, response, err = s.forward(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if status/100 == 2 {
			if err := s.store(key, body, status, response); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}

func (s *memoServer) lookup(key string) (int, []byte, bool, error) {
	var status int
	var response []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		responses := tx.Bucket(responsesBucket)
		statuses := tx.Bucket(statusesBucket)
		if responses == nil || statuses == nil {
			return nil
		}
		stored := responses.Get([]byte(key))
		if stored == nil {
			return nil
		}
		response = append([]byte(nil), stored...)
		code, err := strconv.Atoi(string(statuses.Get([]byte(key))))
		status = code
		return err
	})
	return status, response, response != nil, err
}

func (s *memoServer) store(key string, request []byte, status int, response []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for name, value := range map[string][]byte{
			string(responsesBucket): response,
			string(statusesBucket):  []byte(strconv.Itoa(status)),
			string(requestsBucket):  request,
		} {
			bucket, err := tx.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *memoServer) forward(body []byte) (int, []byte, error) {
	if s.upstream == "" {
		return 0, nil, errors.New("record miss with no -upstream")
	}
	req, err := http.NewRequest(http.MethodPost, s.upstream, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", s.apiVersion)
	if s.apiKey != "" {
		req.Header.Set("x-api-key", s.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(resp.Body)
	return resp.StatusCode, response, err
}

// listenAndWrite listens on address and writes the URL of the listener to
// addressFile, so a runner can pass port 0 and learn the port.
func listenAndWrite(address, addressFile string) (net.Listener, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	if addressFile != "" {
		url := "http://" + ln.Addr().String()
		if err := os.WriteFile(addressFile+".tmp", []byte(url+"\n"), 0o644); err != nil {
			return nil, err
		}
		if err := os.Rename(addressFile+".tmp", addressFile); err != nil {
			return nil, err
		}
	}
	return ln, nil
}

func serveUntilStop(ln net.Listener, handler http.Handler, stop chan struct{}) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(ln) }()
	select {
	case err := <-errs:
		return err
	case <-stop:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	storePath := flags.String("store", "", "bbolt store file")
	mode := flags.String("mode", "replay", "record or replay")
	address := flags.String("listen", "127.0.0.1:0", "listen address")
	addressFile := flags.String("address-file", "", "file that receives the server URL once it listens")
	upstream := flags.String("upstream", "", "real backend URL that record mode forwards a miss to, e.g. https://api.anthropic.com/v1/messages")
	apiKeyEnv := flags.String("api-key-env", "ANTHROPIC_API_KEY", "environment variable holding the upstream API key")
	apiVersion := flags.String("api-version", "2023-06-01", "anthropic-version header sent upstream")
	flags.Parse(args)
	if *storePath == "" {
		return errors.New("serve needs -store")
	}
	if *mode != "record" && *mode != "replay" {
		return fmt.Errorf("unknown mode %q", *mode)
	}
	replay := *mode == "replay"
	if replay {
		if _, err := os.Stat(*storePath); err != nil {
			return err
		}
	}
	db, err := bolt.Open(*storePath, 0o644, &bolt.Options{Timeout: 5 * time.Second, ReadOnly: replay})
	if err != nil {
		return err
	}
	defer db.Close()
	server := &memoServer{
		db:          db,
		replay:      replay,
		upstream:    *upstream,
		apiKey:      os.Getenv(*apiKeyEnv),
		apiVersion:  *apiVersion,
		stopChannel: make(chan struct{}),
	}
	ln, err := listenAndWrite(*address, *addressFile)
	if err != nil {
		return err
	}
	if err := serveUntilStop(ln, server, server.stopChannel); err != nil {
		return err
	}
	if len(server.missedKeys) > 0 {
		return fmt.Errorf("%d replay misses", len(server.missedKeys))
	}
	return nil
}

func runPost(args []string) error {
	flags := flag.NewFlagSet("post", flag.ExitOnError)
	url := flags.String("url", "", "URL to post to")
	bodyPath := flags.String("body", "", "file holding the request body")
	flags.Parse(args)
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		return err
	}
	resp, err := http.Post(*url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("%d\n", resp.StatusCode)
	os.Stdout.Write(response)
	return nil
}

func runStop(args []string) error {
	flags := flag.NewFlagSet("stop", flag.ExitOnError)
	url := flags.String("url", "", "server URL")
	flags.Parse(args)
	resp, err := http.Post(strings.TrimSuffix(*url, "/")+stopPath, "text/plain", nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ExitOnError)
	storePath := flags.String("store", "", "bbolt store file")
	flags.Parse(args)
	db, err := bolt.Open(*storePath, 0o644, &bolt.Options{Timeout: 5 * time.Second, ReadOnly: true})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(func(tx *bolt.Tx) error {
		responses := tx.Bucket(responsesBucket)
		if responses == nil {
			return nil
		}
		statuses := tx.Bucket(statusesBucket)
		requests := tx.Bucket(requestsBucket)
		var keys []string
		responses.ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s status=%s request=%d response=%d\n", k,
				statuses.Get([]byte(k)), len(requests.Get([]byte(k))), len(responses.Get([]byte(k))))
		}
		return nil
	})
}

// The stub is a deterministic stand in for a real model. Its reply depends
// only on the request bytes, so a recording made against it is repeatable.

type stubContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type stubReply struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []stubContentBlock `json:"content"`
	StopReason string             `json:"stop_reason"`
}

type stubRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// StubReplyFor builds the stub reply to an Anthropic Messages request body.
func StubReplyFor(body []byte) ([]byte, error) {
	var req stubRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("no messages")
	}
	tag := RequestKey(body)[:12]
	reply := stubReply{ID: "msg_stub_" + tag, Type: "message", Role: "assistant", Model: req.Model, StopReason: "end_turn"}
	last := req.Messages[len(req.Messages)-1]

	var text string
	if err := json.Unmarshal(last.Content, &text); err != nil {
		var blocks []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(last.Content, &blocks); err != nil {
			return nil, err
		}
		var results []string
		for _, b := range blocks {
			if b.Type == "tool_result" {
				results = append(results, b.Content)
			}
		}
		reply.Content = []stubContentBlock{{Type: "text", Text: fmt.Sprintf(
			"Stub %s: the tools returned %d result(s):\n%s", tag, len(results), strings.Join(results, "\n---\n"))}}
		return marshalStub(reply)
	}

	lower := strings.ToLower(text)
	var calls []stubContentBlock
	if strings.Contains(lower, "time") {
		calls = append(calls, stubContentBlock{Type: "tool_use", ID: "toolu_" + tag + "_time", Name: "get_current_time", Input: json.RawMessage(`{}`)})
	}
	if i := strings.Index(text, "/spiffs/"); i >= 0 {
		path := strings.FieldsFunc(text[i:], func(r rune) bool { return r == ' ' || r == '\n' || r == '?' || r == ',' })[0]
		calls = append(calls, stubContentBlock{Type: "tool_use", ID: "toolu_" + tag + "_read", Name: "read_file", Input: pathInput(path)})
	}
	if len(calls) > 0 {
		reply.StopReason = "tool_use"
		reply.Content = append([]stubContentBlock{{Type: "text", Text: "Let me check."}}, calls...)
		return marshalStub(reply)
	}
	reply.Content = []stubContentBlock{{Type: "text", Text: fmt.Sprintf(
		"Stub %s: you said \"%s\". I see %d message(s) in this conversation.", tag, text, len(req.Messages))}}
	return marshalStub(reply)
}

func pathInput(path string) json.RawMessage {
	input, _ := json.Marshal(map[string]string{"path": path})
	return input
}

func marshalStub(reply stubReply) ([]byte, error) {
	return json.Marshal(reply)
}

type stubServer struct {
	stopChannel chan struct{}
}

func (s *stubServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == stopPath {
		w.WriteHeader(http.StatusOK)
		close(s.stopChannel)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reply, err := StubReplyFor(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(reply)
}

func runStub(args []string) error {
	flags := flag.NewFlagSet("stub", flag.ExitOnError)
	address := flags.String("listen", "127.0.0.1:0", "listen address")
	addressFile := flags.String("address-file", "", "file that receives the server URL once it listens")
	flags.Parse(args)
	ln, err := listenAndWrite(*address, *addressFile)
	if err != nil {
		return err
	}
	server := &stubServer{stopChannel: make(chan struct{})}
	return serveUntilStop(ln, server, server.stopChannel)
}
