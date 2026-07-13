package main

import (
	"fmt"
	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type rulesFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Expr        string            `yaml:"expr"`
			For         string            `yaml:"for"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: promrule-lint <files-or-dirs>")
		os.Exit(2)
	}
	fail := false
	for _, arg := range os.Args[1:] {
		files := []string{arg}
		if st, e := os.Stat(arg); e == nil && st.IsDir() {
			files = nil
			filepath.WalkDir(arg, func(p string, d os.DirEntry, e error) error {
				if e == nil && !d.IsDir() && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")) {
					files = append(files, p)
				}
				return nil
			})
		}
		for _, f := range files {
			if lint(f) {
				fail = true
			}
		}
	}
	if fail {
		os.Exit(1)
	}
}
func lint(path string) bool {
	b, e := os.ReadFile(path)
	if e != nil {
		fmt.Printf("%s: %v\n", path, e)
		return true
	}
	var f rulesFile
	if e = yaml.Unmarshal(b, &f); e != nil {
		fmt.Printf("%s: invalid YAML: %v\n", path, e)
		return true
	}
	bad := false
	for _, g := range f.Groups {
		for _, r := range g.Rules {
			prefix := path + ":" + g.Name + ":" + r.Alert
			if r.Alert == "" {
				continue
			}
			if r.For == "" {
				fmt.Println(prefix + ": missing for duration")
				bad = true
			}
			if r.Annotations["runbook_url"] == "" {
				fmt.Println(prefix + ": missing runbook_url")
				bad = true
			}
			if _, e := parser.ParseExpr(r.Expr); e != nil {
				fmt.Printf("%s: invalid PromQL: %v\n", prefix, e)
				bad = true
			}
			if strings.Contains(r.Expr, "=~\".*\"") {
				fmt.Println(prefix + ": overly broad matcher")
				bad = true
			}
		}
	}
	return bad
}
