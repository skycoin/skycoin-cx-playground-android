// Command portlayout checks the file layout rule R13 of a ported agent: each
// file is named nnn-name.cx, holds one function, one type or one group of
// constant-like variables, and uses only names declared in files that sort
// before it, so lexicographic order is import order and there are no cycles.
//
//	portlayout DIR
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	fileNamePattern = regexp.MustCompile(`^[0-9]{3}-[a-z0-9-]+\.cx$`)
	funcPattern     = regexp.MustCompile(`(?m)^func\s+([A-Za-z_][A-Za-z0-9_]*)`)
	typePattern     = regexp.MustCompile(`(?m)^type\s+([A-Za-z_][A-Za-z0-9_]*)`)
	varPattern      = regexp.MustCompile(`(?m)^var\s+([A-Za-z_][A-Za-z0-9_]*)`)
	identPattern    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	packagePattern  = regexp.MustCompile(`(?m)^package\s+\w+`)
)

type portFile struct {
	name     string
	code     string
	declared []string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: portlayout DIR")
		os.Exit(2)
	}
	paths, err := filepath.Glob(filepath.Join(os.Args[1], "*.cx"))
	if err != nil || len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "portlayout: no .cx files")
		os.Exit(2)
	}
	sort.Strings(paths)

	var files []portFile
	var problems []string
	declaredIn := map[string]int{}
	for i, path := range paths {
		name := filepath.Base(path)
		if !fileNamePattern.MatchString(name) {
			problems = append(problems, name+": name is not nnn-name.cx")
		}
		if want := fmt.Sprintf("%03d-", i); !strings.HasPrefix(name, want) {
			problems = append(problems, name+": expected number "+want)
		}
		source, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		code := stripCommentsAndStrings(string(source))
		code = packagePattern.ReplaceAllString(code, "")
		funcs := funcPattern.FindAllStringSubmatch(code, -1)
		types := typePattern.FindAllStringSubmatch(code, -1)
		vars := varPattern.FindAllStringSubmatch(code, -1)
		single := (len(funcs) == 1 && len(types) == 0 && len(vars) == 0) ||
			(len(types) == 1 && len(funcs) == 0 && len(vars) == 0) ||
			(len(vars) >= 1 && len(funcs) == 0 && len(types) == 0)
		if !single {
			problems = append(problems, fmt.Sprintf("%s: %d funcs, %d types, %d vars; want one func, one type or one var group", name, len(funcs), len(types), len(vars)))
		}
		file := portFile{name: name, code: code}
		for _, group := range [][][]string{funcs, types, vars} {
			for _, m := range group {
				if previous, taken := declaredIn[m[1]]; taken {
					problems = append(problems, fmt.Sprintf("%s: %s already declared in %s", name, m[1], files[previous].name))
				}
				declaredIn[m[1]] = i
				file.declared = append(file.declared, m[1])
			}
		}
		files = append(files, file)
	}

	for i, file := range files {
		seen := map[string]bool{}
		for _, ident := range identPattern.FindAllString(file.code, -1) {
			at, declared := declaredIn[ident]
			if declared && at > i && !seen[ident] {
				seen[ident] = true
				problems = append(problems, fmt.Sprintf("%s uses %s, declared later in %s", file.name, ident, files[at].name))
			}
		}
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Println(p)
		}
		os.Exit(1)
	}
	fmt.Printf("%d files, one declaration each, every name declared before use\n", len(files))
}

// stripCommentsAndStrings blanks line comments and the contents of string
// literals, so names inside them do not count as uses.
func stripCommentsAndStrings(source string) string {
	var out strings.Builder
	for i := 0; i < len(source); i++ {
		switch {
		case source[i] == '/' && i+1 < len(source) && source[i+1] == '/':
			for i < len(source) && source[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
		case source[i] == '"':
			out.WriteString(`""`)
			for i++; i < len(source) && source[i] != '"'; i++ {
				if source[i] == '\\' {
					i++
				}
			}
		default:
			out.WriteByte(source[i])
		}
	}
	return out.String()
}
