package main

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"time"
)

// timingRow is one step of a run. host is measured on the host; device is
// measured from the app's logcat timestamps and is zero when not known.
type timingRow struct {
	step   string
	host   time.Duration
	device time.Duration
	note   string
}

type timingTable struct {
	rows []timingRow
}

func (t *timingTable) add(step string, host, device time.Duration, note string) {
	t.rows = append(t.rows, timingRow{step: step, host: host, device: device, note: note})
}

// timed runs fn and adds a row with its host duration.
func (t *timingTable) timed(step string, fn func() error) error {
	begin := time.Now()
	err := fn()
	note := ""
	if err != nil {
		note = "FAILED"
	}
	t.add(step, time.Since(begin), 0, note)
	return err
}

func millis(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f", float64(d)/float64(time.Millisecond))
}

// write prints the table as aligned plain text.
func (t *timingTable) write(w io.Writer) {
	width := len("step")
	for _, r := range t.rows {
		width = max(width, len(r.step))
	}
	fmt.Fprintf(w, "%-*s  %9s  %9s  %s\n", width, "step", "host ms", "device ms", "note")
	fmt.Fprintf(w, "%s  %9s  %9s  %s\n", strings.Repeat("-", width), "-------", "---------", "----")
	for _, r := range t.rows {
		fmt.Fprintf(w, "%-*s  %9s  %9s  %s\n", width, r.step, millis(r.host), millis(r.device), r.note)
	}
}

func (t *timingTable) String() string {
	var b strings.Builder
	t.write(&b)
	return b.String()
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// differingFraction returns the share of pixels where any channel differs
// by more than tolerance (0..255). Images of different sizes differ fully.
func differingFraction(a, b image.Image, tolerance uint32) float64 {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 1
	}
	ab, bb := a.Bounds(), b.Bounds()
	tol := tolerance * 0x101
	var diff, total int
	for y := 0; y < ab.Dy(); y++ {
		for x := 0; x < ab.Dx(); x++ {
			r1, g1, b1, a1 := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, a2 := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			total++
			if absDiff(r1, r2) > tol || absDiff(g1, g2) > tol || absDiff(b1, b2) > tol || absDiff(a1, a2) > tol {
				diff++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(diff) / float64(total)
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
