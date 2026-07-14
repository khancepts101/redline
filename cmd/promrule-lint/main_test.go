package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRules(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLintValidatesRecordingRulePromQL(t *testing.T) {
	path := writeRules(t, `
groups:
  - name: recording
    rules:
      - record: service:requests:rate5m
        expr: sum(rate(http_requests_total[5m])
`)
	if !lint(path) {
		t.Fatal("invalid recording-rule PromQL was accepted")
	}
}

func TestLintRejectsInvalidForDuration(t *testing.T) {
	path := writeRules(t, `
groups:
  - name: alerts
    rules:
      - alert: BrokenDuration
        expr: up == 0
        for: eventually
        annotations:
          runbook_url: https://example.com/runbook
`)
	if !lint(path) {
		t.Fatal("invalid for duration was accepted")
	}
}

func TestLintFindsBroadMatchersRegardlessOfWhitespace(t *testing.T) {
	for _, expr := range []string{`up{job=~ ".*"}`, `up{job =~".+"}`} {
		t.Run(expr, func(t *testing.T) {
			path := writeRules(t, `
groups:
  - name: alerts
    rules:
      - alert: BroadMatcher
        expr: `+expr+`
        for: 5m
        annotations:
          runbook_url: https://example.com/runbook
`)
			if !lint(path) {
				t.Fatalf("broad matcher in %q was accepted", expr)
			}
		})
	}
}

func TestLintAcceptsValidRecordingAndAlertRules(t *testing.T) {
	path := writeRules(t, `
groups:
  - name: valid
    rules:
      - record: service:requests:rate5m
        expr: sum(rate(http_requests_total{job=~"api-.+"}[5m]))
      - alert: ServiceDown
        expr: up{job="api"} == 0
        for: 5m
        annotations:
          runbook_url: https://example.com/runbook
`)
	if lint(path) {
		t.Fatal("valid rules were rejected")
	}
}
