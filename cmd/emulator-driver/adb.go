package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// device is one Android device reached through adb: the rig's emulator, or
// a phone. The driver talks to it only through adb.
type device struct {
	adb        string // adb binary
	serverPort string // adb server port, kept apart from the user's own server
	serial     string // adb serial, for example localhost:15555
}

func defaultAdb() string {
	if home := os.Getenv("ANDROID_HOME"); home != "" {
		p := filepath.Join(home, "platform-tools", "adb")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "adb"
}

func (d *device) command(ctx context.Context, args ...string) *exec.Cmd {
	full := []string{"-P", d.serverPort}
	if d.serial != "" {
		full = append(full, "-s", d.serial)
	}
	return exec.CommandContext(ctx, d.adb, append(full, args...)...)
}

// adbOutput runs adb and returns its standard output, with standard error
// in the error when it fails.
func (d *device) adbOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := d.command(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("adb %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()+string(out)))
	}
	return out, nil
}

func (d *device) shell(ctx context.Context, args ...string) (string, error) {
	out, err := d.adbOutput(ctx, append([]string{"shell"}, args...)...)
	return strings.TrimSpace(strings.ReplaceAll(string(out), "\r", "")), err
}

// connect attaches a TCP device such as the rig's forwarded adb port.
func (d *device) connect(ctx context.Context) error {
	if !strings.Contains(d.serial, ":") {
		return nil
	}
	out, err := exec.CommandContext(ctx, d.adb, "-P", d.serverPort, "connect", d.serial).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "connected") {
		return fmt.Errorf("adb connect %s: %v: %s", d.serial, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// waitBoot waits until sys.boot_completed is 1.
func (d *device) waitBoot(ctx context.Context) error {
	for {
		if err := d.connect(ctx); err == nil {
			if v, err := d.shell(ctx, "getprop", "sys.boot_completed"); err == nil && v == "1" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for boot: %w", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (d *device) install(ctx context.Context, apk string) error {
	_, err := d.adbOutput(ctx, "install", "-r", "-g", apk)
	return err
}

func (d *device) uninstall(ctx context.Context, appID string) error {
	_, err := d.adbOutput(ctx, "uninstall", appID)
	return err
}

// startApp launches the app's activity and waits until it is shown. The
// test properties must be set before, as the app reads them at launch.
func (d *device) startApp(ctx context.Context, appID, autostart string) error {
	if _, err := d.shell(ctx, "setprop", "debug.cxplayground.test", "1"); err != nil {
		return err
	}
	// setprop cannot set an empty value, so a space means none.
	if autostart == "" {
		autostart = " "
	}
	if _, err := d.shell(ctx, "setprop", "debug.cxplayground.autostart", shellQuote(autostart)); err != nil {
		return err
	}
	out, err := d.shell(ctx, "am", "start", "-W", "-n", appID+"/org.gioui.GioActivity")
	if err == nil && strings.Contains(out, "Error") {
		err = fmt.Errorf("am start: %s", out)
	}
	return err
}

func (d *device) stopApp(ctx context.Context, appID string) error {
	_, err := d.shell(ctx, "am", "force-stop", appID)
	return err
}

func (d *device) screenshot(ctx context.Context) ([]byte, error) {
	png, err := d.adbOutput(ctx, "exec-out", "screencap", "-p")
	if err == nil && !bytes.HasPrefix(png, []byte("\x89PNG")) {
		err = fmt.Errorf("screencap did not return a PNG: %.80q", png)
	}
	return png, err
}

func (d *device) clearLog(ctx context.Context) error {
	_, err := d.adbOutput(ctx, "logcat", "-c")
	return err
}

// appLog returns the logcat lines of the app's tag, which carries the Go
// program's log, standard output and standard error.
func (d *device) appLog(ctx context.Context, appID string) (string, error) {
	out, err := d.adbOutput(ctx, "logcat", "-d", "-v", "epoch", "-s", appID+":V")
	return string(out), err
}

// shellQuote quotes s for the device shell that adb shell runs.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
