// Command emulator-driver drives the CX playground app on an Android device
// through adb: the rig's headless emulator from docker/emulator, or a phone.
//
//	emulator-driver boot                 start the rig container, wait for boot
//	emulator-driver build                build the x86_64 APK with make apk
//	emulator-driver install <apk>        install an APK
//	emulator-driver run [-app name]      start the app in test mode
//	emulator-driver inject <step>        send one input.txt step, e.g. "key ENTER"
//	emulator-driver screenshot -o f.png  save the screen
//	emulator-driver log                  print the app's logcat
//	emulator-driver uninstall            remove the app
//	emulator-driver stop                 stop and remove the rig container
//	emulator-driver suite [tests/x ...]  run the emulator test folders, print the timing table
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type options struct {
	dev     device
	rig     rigContainer
	appID   string
	repo    string
	apk     string
	results string
}

func (o *options) register(fs *flag.FlagSet) {
	fs.StringVar(&o.dev.adb, "adb", defaultAdb(), "adb binary")
	fs.StringVar(&o.dev.serverPort, "adb-server-port", "5038", "adb server port, apart from the default 5037 so the user's own adb server is left alone")
	fs.StringVar(&o.dev.serial, "serial", "localhost:15555", "adb serial of the device")
	fs.StringVar(&o.rig.image, "image", "cx-android-rig", "rig docker image")
	fs.StringVar(&o.rig.name, "container", "cx-android-rig", "rig container name")
	fs.StringVar(&o.rig.memory, "memory", "4g", "rig container memory limit")
	fs.StringVar(&o.rig.adbPort, "adb-port", "15555", "host port for the emulator's adb")
	fs.StringVar(&o.rig.console, "console-port", "15554", "host port for the emulator console")
	fs.StringVar(&o.appID, "app-id", "com.skycoin.cxplayground", "Android package of the app")
	fs.StringVar(&o.repo, "repo", ".", "playground repository, for make apk")
	fs.StringVar(&o.apk, "apk", "build/cx-playground-x86_64.apk", "APK path, relative to -repo")
	fs.StringVar(&o.results, "results", "results", "directory for run results")
}

func (o *options) apkPath() string {
	if filepath.IsAbs(o.apk) {
		return o.apk
	}
	return filepath.Join(o.repo, o.apk)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runCommand(ctx, os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "emulator-driver:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: emulator-driver boot|build|install|run|inject|screenshot|log|uninstall|stop|suite [flags]")
	os.Exit(2)
}

func runCommand(ctx context.Context, name string, args []string) error {
	var o options
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	o.register(fs)
	appName := fs.String("app", "", "run: manifest name of the CX app to autostart")
	shotFile := fs.String("o", "screen.png", "screenshot: output file")
	boot := fs.Bool("boot", false, "suite: start the rig container first")
	stopAfter := fs.Bool("stop", false, "suite: stop the rig container at the end")
	skipBuild := fs.Bool("skip-build", false, "suite: use the existing -apk")
	tolerance := fs.Uint("tolerance", 16, "suite: per channel tolerance of screenshot comparison, 0..255")
	maxDiff := fs.Float64("max-diff", 0.01, "suite: share of pixels that may differ for screen-matches")
	updateReferences := fs.Bool("update-references", false, "suite: write the screen-matches captures to expected/ instead of comparing them")
	fs.Parse(args)
	d := &o.dev

	switch name {
	case "boot":
		res, err := o.rig.start(ctx, d)
		if err != nil {
			return err
		}
		fmt.Printf("boot completed: emulator %d ms, host (docker run to adb) %d ms\n", res.emulator.Milliseconds(), res.host.Milliseconds())
		begin := time.Now()
		if err := d.waitSettled(ctx, time.Minute); err != nil {
			return err
		}
		fmt.Printf("settled %d ms later\n", time.Since(begin).Milliseconds())
		return nil
	case "stop":
		return o.stopRig(ctx)
	case "build":
		begin := time.Now()
		if err := buildAPK(ctx, o.repo, o.apk); err != nil {
			return err
		}
		fmt.Printf("%s built in %d ms\n", o.apkPath(), time.Since(begin).Milliseconds())
		return nil
	case "install":
		apk := o.apkPath()
		if fs.NArg() > 0 {
			apk = fs.Arg(0)
		}
		return d.install(ctx, apk)
	case "uninstall":
		return d.uninstall(ctx, o.appID)
	case "run":
		return d.startApp(ctx, o.appID, *appName)
	case "inject":
		steps, err := parseSteps(strings.NewReader(strings.Join(fs.Args(), " ")))
		if err != nil {
			return err
		}
		if len(steps) != 1 || !steps[0].injected() {
			return fmt.Errorf("inject takes one text, key, tap or swipe step")
		}
		_, err = d.shell(ctx, steps[0].inputCommand()...)
		return err
	case "screenshot":
		png, err := d.screenshot(ctx)
		if err != nil {
			return err
		}
		return os.WriteFile(*shotFile, png, 0o644)
	case "log":
		out, err := d.appLog(ctx, o.appID)
		fmt.Print(out)
		return err
	case "suite":
		return o.suite(ctx, fs.Args(), *boot, *stopAfter, *skipBuild, &session{tolerance: uint32(*tolerance), maxDiff: *maxDiff, updateReferences: *updateReferences})
	}
	usage()
	return nil
}

// stopRig removes the rig container and the driver's own adb server.
func (o *options) stopRig(ctx context.Context) error {
	if o.dev.serverPort != "5037" {
		server := o.dev
		server.serial = ""
		server.adbOutput(ctx, "kill-server")
	}
	return o.rig.stop(ctx)
}

func (o *options) suite(ctx context.Context, dirs []string, boot, stopAfter, skipBuild bool, s *session) (err error) {
	if len(dirs) == 0 {
		dirs, _ = filepath.Glob(filepath.Join(o.repo, "tests", "*"))
	}
	var tests []testFolder
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t, err := loadTestFolder(dir)
		if errors.Is(err, errNotEmulatorTest) {
			continue
		}
		if err != nil {
			return err
		}
		tests = append(tests, t)
	}
	if len(tests) == 0 {
		return fmt.Errorf("no test folders")
	}
	stamp := time.Now().Format("20060102-150405")
	s.dev, s.appID, s.results = &o.dev, o.appID, filepath.Join(o.results, stamp)
	s.rig, s.repo = &o.rig, o.repo
	if err := os.MkdirAll(s.results, 0o755); err != nil {
		return err
	}
	if stopAfter {
		defer func() {
			if e := o.stopRig(context.Background()); e != nil && err == nil {
				err = e
			}
		}()
	}

	var shared timingTable
	runBegin := time.Now()
	if boot {
		res, err := o.rig.start(ctx, &o.dev)
		if err != nil {
			return err
		}
		shared.add("boot (docker run to boot completed)", res.host, res.emulator, "device ms: emulator start to boot completed")
	} else if err := o.dev.waitBoot(ctx); err != nil {
		return err
	}
	if err := shared.timed("settle (home focused, theme overlays on)", func() error { return o.dev.waitSettled(ctx, time.Minute) }); err != nil {
		return err
	}
	if !skipBuild {
		if err := shared.timed("compile and package (make apk, gogio)", func() error { return buildAPK(ctx, o.repo, o.apk) }); err != nil {
			return err
		}
	}

	var report strings.Builder
	failed := 0
	for _, t := range tests {
		var table timingTable
		failures := s.runTest(ctx, t, o.apkPath(), &table)
		os.WriteFile(filepath.Join(s.results, t.name, "timing.txt"), []byte(table.String()), 0o644)
		verdict := "PASS"
		if len(failures) > 0 {
			verdict = "FAIL"
			failed++
		}
		fmt.Fprintf(&report, "\n%s  %s\n", verdict, t.name)
		for _, f := range failures {
			fmt.Fprintf(&report, "  %s\n", f)
		}
		shared.rows = append(shared.rows, timingRow{step: "[" + t.name + "]"})
		shared.rows = append(shared.rows, table.rows...)
	}
	shared.add("run total", time.Since(runBegin), 0, "")
	fmt.Fprintf(&report, "\n%s\n%d of %d tests passed, results in %s\n", shared.String(), len(tests)-failed, len(tests), s.results)
	os.WriteFile(filepath.Join(s.results, "timing.txt"), []byte(shared.String()), 0o644)
	fmt.Print(report.String())
	if failed > 0 {
		return fmt.Errorf("%d tests failed", failed)
	}
	return nil
}
