package cxir

import "errors"

// Lines builds a function body with named labels instead of the relative
// line offsets that OpJmp and OpGoto carry. It plays the part of the CX
// parser, which turns if and for into those jumps.
type Lines struct {
	exprs  []Expr
	labels map[string]int
	fixes  []fix
}

type fix struct {
	line        int
	then, other string
}

// Label names the next line.
func (l *Lines) Label(name string) {
	if l.labels == nil {
		l.labels = map[string]int{}
	}
	l.labels[name] = len(l.exprs)
}

// Add appends lines.
func (l *Lines) Add(e ...Expr) { l.exprs = append(l.exprs, e...) }

// If jumps to then when cond is true and to other otherwise. An empty label
// means the next line.
func (l *Lines) If(cond Arg, then, other string) {
	l.fixes = append(l.fixes, fix{len(l.exprs), then, other})
	l.exprs = append(l.exprs, Jmp(cond, 0, 0))
}

// Goto jumps to label.
func (l *Lines) Goto(label string) {
	l.fixes = append(l.fixes, fix{line: len(l.exprs), then: label})
	l.exprs = append(l.exprs, Goto(0))
}

// Exprs resolves the labels and returns the lines.
func (l *Lines) Exprs() ([]Expr, error) {
	off := func(line int, label string) (int, error) {
		if label == "" {
			return 0, nil
		}
		t, ok := l.labels[label]
		if !ok {
			return 0, errors.New("undefined label " + label)
		}
		return t - line - 1, nil
	}
	for _, f := range l.fixes {
		var err1, err2 error
		e := &l.exprs[f.line]
		e.ThenLines, err1 = off(f.line, f.then)
		if e.Op == OpJmp {
			e.ElseLines, err2 = off(f.line, f.other)
		}
		if err := errors.Join(err1, err2); err != nil {
			return nil, err
		}
	}
	return l.exprs, nil
}
