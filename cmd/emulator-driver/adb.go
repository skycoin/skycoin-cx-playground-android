package main

import (
	"bytes"
	"context"
	"errors"
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

// keyboardShown reports whether the soft keyboard is on screen.
func (d *device) keyboardShown(ctx context.Context) (bool, error) {
	out, err := d.shell(ctx, "dumpsys", "input_method")
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "mInputShown=true"), nil
}

// waitKeyboard waits until the soft keyboard is shown or hidden.
func (d *device) waitKeyboard(ctx context.Context, shown bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		got, err := d.keyboardShown(ctx)
		if err == nil && got == shown {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("soft keyboard shown=%v after %v, want %v (%v)", got, timeout, shown, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitSettled waits after boot until a home screen has window focus and the
// SystemUI theme overlays are on. SystemUI turns them on seconds after
// sys.boot_completed and every app relaunches then, which loses the keys sent
// meanwhile. A device without the overlays is taken as settled after timeout.
func (d *device) waitSettled(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		focus := currentFocus(d.shellOrEmpty(ctx, "dumpsys", "window"))
		home := focus != "" && focus != "null" && !strings.Contains(focus, "FallbackHome")
		if home && strings.Contains(d.shellOrEmpty(ctx, "cmd", "overlay", "list"), "[x] com.android.systemui:") {
			return nil
		}
		if time.Now().After(deadline) {
			if home {
				return nil
			}
			return fmt.Errorf("no home screen focused after %v, focus %q", timeout, focus)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (d *device) shellOrEmpty(ctx context.Context, args ...string) string {
	out, _ := d.shell(ctx, args...)
	return out
}

// currentFocus returns the window of the mCurrentFocus line of dumpsys window.
func currentFocus(dump string) string {
	_, rest, ok := strings.Cut(dump, "mCurrentFocus=")
	if !ok {
		return ""
	}
	line, _, _ := strings.Cut(rest, "\n")
	return strings.TrimSpace(line)
}

// waitWindowFocus waits until the resumed activity of appID has window focus
// in its own process. Android drops key events that reach a window before
// its ViewRootImpl has seen the focus (ViewRootImpl.shouldDropInputEvent),
// which happens while a cold started app is busy on its main thread.
func (d *device) waitWindowFocus(ctx context.Context, appID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if windowFocused(d.shellOrEmpty(ctx, "dumpsys", "activity", "top"), appID) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s has no window focus after %v", appID, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// windowFocused reports whether dumpsys activity top shows a resumed
// activity of appID whose view root has window focus.
func windowFocused(dump, appID string) bool {
	for _, a := range strings.Split(dump, "ACTIVITY ")[1:] {
		if strings.HasPrefix(a, appID+"/") && strings.Contains(a, "mResumed=true") && strings.Contains(a, "mHasWindowFocus=true") {
			return true
		}
	}
	return false
}

// leaveTouchMode takes the display out of touch mode, which a boot or a tap
// puts it in. A window created in touch mode loses the first navigation key
// (ENTER, a DPAD key, TAB), which is consumed to leave touch mode
// (ViewRootImpl checkForLeavingTouchModeAndConsume). The driver spends that
// with PAGE_UP, a navigation key that the app ignores, sent to the app's own
// window, since another window may not know the display is in touch mode.
func (d *device) leaveTouchMode(ctx context.Context) error {
	inTouchMode := func() bool { return strings.Contains(d.shellOrEmpty(ctx, "dumpsys", "window"), "mInTouchMode=true") }
	for range 3 {
		if !inTouchMode() {
			return nil
		}
		if _, err := d.shell(ctx, "input", "keyevent", "KEYCODE_PAGE_UP"); err != nil {
			return err
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && inTouchMode() {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if inTouchMode() {
		return errors.New("the display stayed in touch mode after 3 PAGE_UP keys")
	}
	return nil
}
