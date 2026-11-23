package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// rigContainer is the docker container that runs the emulator image from
// docker/emulator.
type rigContainer struct {
	image   string
	name    string
	memory  string // docker --memory, also used as --memory-swap
	adbPort string // host port for the emulator's adb
	console string // host port for the emulator console
}

// bootResult holds the two boot times: the one the container reports for
// the emulator alone, and the one the host saw from docker run to boot
// completed over adb.
type bootResult struct {
	emulator time.Duration
	host     time.Duration
}

var bootLine = regexp.MustCompile(`rig: boot completed in (\d+) ms`)

func (r *rigContainer) start(ctx context.Context, d *device) (bootResult, error) {
	var res bootResult
	begin := time.Now()
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--name", r.name,
		"--device", "/dev/kvm", "--cap-add", "NET_ADMIN", "--memory", r.memory, "--memory-swap", r.memory,
		"-p", "127.0.0.1:"+r.adbPort+":15555", "-p", "127.0.0.1:"+r.console+":15554",
		r.image).CombinedOutput()
	if err != nil {
		return res, fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	for {
		logs, _ := exec.CommandContext(ctx, "docker", "logs", r.name).CombinedOutput()
		if m := bootLine.FindSubmatch(logs); m != nil && strings.Contains(string(logs), "rig: ready") {
			var ms int64
			fmt.Sscan(string(m[1]), &ms)
			res.emulator = time.Duration(ms) * time.Millisecond
			break
		}
		state, _ := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", r.name).Output()
		if strings.TrimSpace(string(state)) == "false" {
			return res, fmt.Errorf("rig container exited:\n%s", logs)
		}
		select {
		case <-ctx.Done():
			return res, fmt.Errorf("waiting for the rig: %w\n%s", ctx.Err(), logs)
		case <-time.After(time.Second):
		}
	}
	if err := d.waitBoot(ctx); err != nil {
		return res, err
	}
	res.host = time.Since(begin)
	return res, nil
}

// stop stops and removes the container. It touches no other container.
func (r *rigContainer) stop(ctx context.Context) error {
	exec.CommandContext(ctx, "docker", "stop", "-t", "10", r.name).Run()
	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", r.name).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "No such container") {
		return fmt.Errorf("docker rm %s: %v: %s", r.name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
