package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The network settings of emulator.txt work in the rig container, which is
// the emulator's host: the emulator reaches it as 10.0.2.2 through its user
// mode network, and that traffic reaches the echo server over the
// container's loopback, where tc and tcpdump act on it.
//
//	echo-server <port>   runs net-echo-server on the TCP and UDP port
//	netem <args>         tc netem on the port's packets, e.g. delay 200ms loss 30%
//	capture <file>       tcpdump of the port's packets, pulled as <file>
type netSettings struct {
	echoPort string
	netem    string
	capture  string
}

func (n netSettings) used() bool { return n.echoPort != "" || n.netem != "" || n.capture != "" }

// dockerExec runs a command in the rig container.
func (s *session) dockerExec(ctx context.Context, detach bool, args ...string) (string, error) {
	full := []string{"exec"}
	if detach {
		full = append(full, "-d")
	}
	full = append(append(full, s.rig.name), args...)
	out, err := exec.CommandContext(ctx, "docker", full...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %v: %s", strings.Join(full, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// echoServerBinary builds net-echo-server for the container once per run.
func (s *session) echoServerBinary(ctx context.Context) (string, error) {
	path := filepath.Join(s.results, "net-echo-server")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", abs, "./cmd/net-echo-server")
	cmd.Dir = s.repo
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build net-echo-server: %v: %s", err, out)
	}
	return abs, nil
}

// netSetup starts what the settings ask for and returns the function that
// undoes it and returns the path of the capture, if any.
func (s *session) netSetup(ctx context.Context, t testFolder, out string) (func() (string, error), error) {
	n := t.net
	if s.rig == nil || s.rig.name == "" {
		return nil, errors.New("the network settings need the rig container")
	}
	port := n.echoPort
	if port == "" {
		return nil, errors.New("netem and capture need echo-server <port>")
	}
	var undo []func() error
	stop := func() (string, error) {
		var errs []error
		for i := len(undo) - 1; i >= 0; i-- {
			errs = append(errs, undo[i]())
		}
		path := ""
		if n.capture != "" {
			path = filepath.Join(out, n.capture)
			if b, err := exec.CommandContext(ctx, "docker", "cp", s.rig.name+":/tmp/"+n.capture, path).CombinedOutput(); err != nil {
				errs = append(errs, fmt.Errorf("docker cp %s: %v: %s", n.capture, err, b))
				path = ""
			}
		}
		return path, errors.Join(errs...)
	}

	bin, err := s.echoServerBinary(ctx)
	if err != nil {
		return nil, err
	}
	if b, err := exec.CommandContext(ctx, "docker", "cp", bin, s.rig.name+":/tmp/net-echo-server").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker cp net-echo-server: %v: %s", err, b)
	}
	// A server from an earlier test may hold the port already, then this one exits.
	if _, err := s.dockerExec(ctx, true, "/tmp/net-echo-server", "-addr", "0.0.0.0:"+port); err != nil {
		return nil, err
	}
	if err := s.waitContainer(ctx, "ss -ltn | grep -q ':"+port+" '"); err != nil {
		return nil, fmt.Errorf("echo server not listening on %s: %v", port, err)
	}

	if n.netem != "" {
		match := func(dir string) []string {
			return []string{"tc", "filter", "add", "dev", "lo", "parent", "1:", "protocol", "ip", "prio", "1", "u32", "match", "ip", dir, port, "0xffff", "flowid", "1:3"}
		}
		for _, c := range [][]string{
			{"tc", "qdisc", "add", "dev", "lo", "root", "handle", "1:", "prio"},
			append([]string{"tc", "qdisc", "add", "dev", "lo", "parent", "1:3", "handle", "30:", "netem"}, strings.Fields(n.netem)...),
			match("dport"), match("sport"),
		} {
			if _, err := s.dockerExec(ctx, false, c...); err != nil {
				s.dockerExec(ctx, false, "tc", "qdisc", "del", "dev", "lo", "root")
				return nil, err
			}
		}
		undo = append(undo, func() error {
			_, err := s.dockerExec(ctx, false, "tc", "qdisc", "del", "dev", "lo", "root")
			return err
		})
	}
	if n.capture != "" {
		file := "/tmp/" + n.capture
		script := fmt.Sprintf("rm -f %[1]s %[1]s.pid; tcpdump -i lo -U -w %[1]s port %[2]s 2>%[1]s.log & echo $! > %[1]s.pid; wait", file, port)
		if _, err := s.dockerExec(ctx, true, "sh", "-c", script); err != nil {
			stop()
			return nil, err
		}
		if err := s.waitContainer(ctx, "grep -q listening "+file+".log"); err != nil {
			stop()
			return nil, fmt.Errorf("tcpdump did not start: %v", err)
		}
		undo = append(undo, func() error {
			_, err := s.dockerExec(ctx, false, "sh", "-c", "kill $(cat "+file+".pid); sleep 0.3")
			return err
		})
	}
	return stop, nil
}

// waitContainer waits up to 5 s for a shell condition in the container.
func (s *session) waitContainer(ctx context.Context, cond string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := s.dockerExec(ctx, false, "sh", "-c", cond)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}
