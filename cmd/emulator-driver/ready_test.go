package main

import (
	"fmt"
	"testing"
)

func TestWindowFocused(t *testing.T) {
	dump := `TASK 10074:com.android.launcher3 id=5
  ACTIVITY com.android.launcher3/.Launcher 1 pid=900
    mResumed=false mStopped=true mFinished=false
      mHasWindowFocus=false
TASK 10149:com.skycoin.cxplayground id=8
  ACTIVITY com.skycoin.cxplayground/org.gioui.GioActivity 2 pid=3135
    mResumed=true mStopped=false mFinished=false
        mHasWindowFocus=%s
`
	app := "com.skycoin.cxplayground"
	for _, c := range []struct {
		focus string
		want  bool
	}{{"true", true}, {"false", false}} {
		if got := windowFocused(fmt.Sprintf(dump, c.focus), app); got != c.want {
			t.Errorf("focus %s: got %v", c.focus, got)
		}
	}
	if windowFocused(fmt.Sprintf(dump, "true"), "com.other") {
		t.Error("another app counted as focused")
	}
}

func TestCurrentFocus(t *testing.T) {
	dump := "  mCurrentFocus=Window{f7081c6 u0 com.android.launcher3/com.android.launcher3.uioverrides.QuickstepLauncher}\n  mFocusedApp=x\n"
	if got := currentFocus(dump); got != "Window{f7081c6 u0 com.android.launcher3/com.android.launcher3.uioverrides.QuickstepLauncher}" {
		t.Errorf("got %q", got)
	}
	if currentFocus("nothing") != "" {
		t.Error("no line gave a focus")
	}
}

func TestLastInputFocus(t *testing.T) {
	lines := []logLine{{trace: "input focused=true"}, {trace: "frame n=1 event=0"}, {trace: "input focused=false"}}
	if lastInputFocus(lines) {
		t.Error("the latest line says false")
	}
	if !lastInputFocus(lines[:2]) {
		t.Error("the latest line says true")
	}
}
