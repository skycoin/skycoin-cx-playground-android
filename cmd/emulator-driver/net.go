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
//	program-server <port> <dir> <seed>
//	                     runs programblocks serve on the UDP port for the program
//	                     directories under <dir>, relative to the test folder,
//	                     signed with the key derived from <seed>
//	netem <args>         tc netem on the port's packets, e.g. delay 200ms loss 30%
//	capture <file>       tcpdump of the port's packets, pulled as <file>
type netSettings struct {
	echoPort    string
	programPort string
	programDir  string
	programSeed string
	netem       string
	capture     string
}

func (n netSettings) used() bool {
	return n.echoPort != "" || n.programPort != "" || n.netem != "" || n.capture != ""
}

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

// hostBinary builds ./cmd/<name> for the container once per run.
func (s *session) hostBinary(ctx context.Context, name string) (string, error) {
	path := filepath.Join(s.results, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", abs, "./cmd/"+name)
	cmd.Dir = s.repo
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build %s: %v: %s", name, err, out)
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
		port = n.programPort
	}
	if port == "" {
		return nil, errors.New("netem and capture need echo-server <port> or program-server")
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

	if n.echoPort != "" {
		bin, err := s.hostBinary(ctx, "net-echo-server")
		if err != nil {
			return nil, err
		}
		if b, err := exec.CommandContext(ctx, "docker", "cp", bin, s.rig.name+":/tmp/net-echo-server").CombinedOutput(); err != nil {
			return nil, fmt.Errorf("docker cp net-echo-server: %v: %s", err, b)
		}
		// A server from an earlier test may hold the port already, then this one exits.
		if _, err := s.dockerExec(ctx, true, "/tmp/net-echo-server", "-addr", "0.0.0.0:"+n.echoPort); err != nil {
			return nil, err
		}
		if err := s.waitContainer(ctx, "ss -ltn | grep -q ':"+n.echoPort+" '"); err != nil {
			return nil, fmt.Errorf("echo server not listening on %s: %v", n.echoPort, err)
		}
	}
	if n.programPort != "" {
		if err := s.startProgramServer(ctx, n, &undo); err != nil {
			stop()
			return nil, err
		}
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

// startProgramServer copies programblocks and the test's program
// directories into the container and serves them on the program port. The
// server is stopped when the test's network settings are undone.
func (s *session) startProgramServer(ctx context.Context, n netSettings, undo *[]func() error) error {
	bin, err := s.hostBinary(ctx, "programblocks")
	if err != nil {
		return err
	}
	dir := "/tmp/programs-" + n.programPort
	if _, err := s.dockerExec(ctx, false, "rm", "-rf", dir); err != nil {
		return err
	}
	for _, c := range [][2]string{{bin, "/tmp/programblocks"}, {n.programDir, dir}} {
		if b, err := exec.CommandContext(ctx, "docker", "cp", c[0], s.rig.name+":"+c[1]).CombinedOutput(); err != nil {
			return fmt.Errorf("docker cp %s: %v: %s", c[0], err, b)
		}
	}
	pid := dir + ".pid"
	script := fmt.Sprintf("/tmp/programblocks serve -addr 0.0.0.0:%s -seed %s %s > %s.log 2>&1 & echo $! > %s; wait",
		n.programPort, shellQuote(n.programSeed), dir, dir, pid)
	if _, err := s.dockerExec(ctx, true, "sh", "-c", script); err != nil {
		return err
	}
	if err := s.waitContainer(ctx, "grep -q serving "+dir+".log"); err != nil {
		log, _ := s.dockerExec(ctx, false, "cat", dir+".log")
		return fmt.Errorf("program server did not start: %v: %s", err, log)
	}
	*undo = append(*undo, func() error {
		_, err := s.dockerExec(ctx, false, "sh", "-c", "kill $(cat "+pid+")")
		return err
	})
	return nil
}
