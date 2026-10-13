package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

const sample = `
jobs:
  check:
    steps:
      - uses: actions/checkout@v1
      - name: first
        run: echo one
      - run: echo "$GREETING"
        env:
          GREETING: hi
  other:
    steps:
      - run: echo other
`

func TestLoadSteps_SkipsUsesAndKeepsOrder(t *testing.T) {
	steps, err := loadSteps([]byte(sample), "check")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Name != "first" || steps[1].Name != "step 3" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestLoadSteps_Errors(t *testing.T) {
	tests := map[string]struct {
		yaml, want string
	}{
		"unknown job": {sample, `no job "missing" (jobs: check, other)`},
		"expression":  {"jobs: {missing: {steps: [{run: 'echo ${{ github.sha }}'}]}}", "expression"},
		"shell":       {"jobs: {missing: {steps: [{run: x, shell: pwsh}]}}", "shell"},
		"workdir":     {"jobs: {missing: {steps: [{run: x, working-directory: a}]}}", "working-directory"},
		"env expr":    {"jobs: {missing: {steps: [{run: x, env: {A: '${{ x }}'}}]}}", "env A"},
		"no run":      {"jobs: {missing: {steps: [{uses: a}]}}", "no run: steps"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadSteps([]byte(tc.yaml), "missing")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestRunSteps_EnvAndStopOnFailure(t *testing.T) {
	steps := []step{
		{Name: "env", Run: `echo "$GREETING"; test -n "$GITHUB_STEP_SUMMARY"`, Env: map[string]string{"GREETING": "hi"}},
		{Name: "pipefail", Run: "false | true"},
		{Name: "never", Run: "echo never"},
	}
	var out bytes.Buffer
	err := runSteps(steps, &out, &out)
	if err == nil || !strings.Contains(err.Error(), `"pipefail"`) {
		t.Fatalf("err = %v, want the pipefail step to fail", err)
	}
	if !strings.Contains(out.String(), "hi\n") || strings.Contains(out.String(), "never") {
		t.Fatalf("output:\n%s", out.String())
	}
}

// The real workflow must stay runnable locally.
func TestCIWorkflowIsRunnableLocally(t *testing.T) {
	data, err := os.ReadFile("../../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadSteps(data, "check"); err != nil {
		t.Fatal(err)
	}
}
