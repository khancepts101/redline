package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"
)

type rule struct {
	Alert       string            `yaml:"alert"`
	Record      string            `yaml:"record"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Annotations map[string]string `yaml:"annotations"`
}

type rulesFile struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []rule `yaml:"rules"`
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
			name := r.Alert
			if name == "" {
				name = r.Record
			}
			if name == "" {
				name = "<unnamed>"
			}
			prefix := path + ":" + g.Name + ":" + name

			expr, err := parser.ParseExpr(r.Expr)
			if err != nil {
				fmt.Printf("%s: invalid PromQL: %v\n", prefix, err)
				bad = true
			} else if hasBroadMatcher(expr) {
				fmt.Println(prefix + ": overly broad matcher")
				bad = true
			}

			if r.Alert != "" {
				if r.For == "" {
					fmt.Println(prefix + ": missing for duration")
					bad = true
				} else if _, err := model.ParseDuration(r.For); err != nil {
					fmt.Printf("%s: invalid for duration %q: %v\n", prefix, r.For, err)
					bad = true
				}
				if r.Annotations["runbook_url"] == "" {
					fmt.Println(prefix + ": missing runbook_url")
					bad = true
				}
			}
		}
	}
	return bad
}

func hasBroadMatcher(expr parser.Expr) bool {
	broad := false
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		selector, ok := node.(*parser.VectorSelector)
		if !ok {
			return nil
		}
		for _, matcher := range selector.LabelMatchers {
			if matcher.Type == labels.MatchRegexp && (matcher.Value == ".*" || matcher.Value == ".+") {
				broad = true
			}
		}
		return nil
	})
	return broad
}
