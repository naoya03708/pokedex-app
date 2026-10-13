// Command cilocal runs the run: steps of a GitHub Actions job locally, so
// that the workflow file stays the single definition of the checks.
//
//	go run ./internal/tools/cilocal            # job "check" of ci.yml
//	go run ./internal/tools/cilocal -list      # print the steps only
//
// Steps with uses: are skipped (checkout, setup-go). Each run: step runs
// with the same shell GitHub uses on Linux. GITHUB_STEP_SUMMARY points to
// a temporary file, so summary steps work but write nowhere visible. A step
// that relies on something only Actions provides (an expression, another
// shell, a working directory) is an error rather than a silent difference.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type workflow struct {
	Jobs map[string]struct {
		Steps []step `yaml:"steps"`
	} `yaml:"jobs"`
}

type step struct {
	Name             string            `yaml:"name"`
	Uses             string            `yaml:"uses"`
	Run              string            `yaml:"run"`
	Env              map[string]string `yaml:"env"`
	Shell            string            `yaml:"shell"`
	WorkingDirectory string            `yaml:"working-directory"`
}

// loadSteps returns the steps of job in the workflow data that run
// locally, in order.
func loadSteps(data []byte, job string) ([]step, error) {
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	j, ok := wf.Jobs[job]
	if !ok {
		names := make([]string, 0, len(wf.Jobs))
		for n := range wf.Jobs {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("no job %q (jobs: %s)", job, strings.Join(names, ", "))
	}
	var steps []step
	for i, s := range j.Steps {
		if s.Run == "" {
			continue
		}
		if s.Name == "" {
			s.Name = fmt.Sprintf("step %d", i+1)
		}
		switch {
		case strings.Contains(s.Run, "${{"):
			return nil, fmt.Errorf("%s: run uses a ${{ }} expression, which only Actions can evaluate", s.Name)
		case s.Shell != "":
			return nil, fmt.Errorf("%s: shell %q is not supported locally", s.Name, s.Shell)
		case s.WorkingDirectory != "":
			return nil, fmt.Errorf("%s: working-directory is not supported locally", s.Name)
		}
		for k, v := range s.Env {
			if strings.Contains(v, "${{") {
				return nil, fmt.Errorf("%s: env %s uses a ${{ }} expression", s.Name, k)
			}
		}
		steps = append(steps, s)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("job %q has no run: steps", job)
	}
	return steps, nil
}

// runSteps runs steps in order and stops at the first failure.
func runSteps(steps []step, stdout, stderr io.Writer) error {
	summary, err := os.CreateTemp("", "cilocal-summary-*")
	if err != nil {
		return err
	}
	summary.Close()
	defer os.Remove(summary.Name())

	for i, s := range steps {
		fmt.Fprintf(stdout, "==> [%d/%d] %s\n", i+1, len(steps), s.Name)
		// The default shell of Actions on Linux.
		cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", s.Run)
		cmd.Stdout, cmd.Stderr = stdout, stderr
		cmd.Env = append(os.Environ(), "CI=true", "GITHUB_STEP_SUMMARY="+summary.Name())
		for k, v := range s.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("step %q failed: %w", s.Name, err)
		}
	}
	fmt.Fprintf(stdout, "==> all %d steps passed\n", len(steps))
	return nil
}

func main() {
	file := flag.String("f", ".github/workflows/ci.yml", "workflow file")
	job := flag.String("job", "check", "job whose steps to run")
	list := flag.Bool("list", false, "print the steps without running them")
	flag.Parse()

	data, err := os.ReadFile(*file)
	if err == nil {
		var steps []step
		if steps, err = loadSteps(data, *job); err == nil {
			if *list {
				for _, s := range steps {
					fmt.Printf("%s:\n  %s\n", s.Name, strings.ReplaceAll(strings.TrimSpace(s.Run), "\n", "\n  "))
				}
				return
			}
			err = runSteps(steps, os.Stdout, os.Stderr)
		}
	}
	if err != nil {
		var exit *exec.ExitError
		fmt.Fprintln(os.Stderr, "cilocal:", err)
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
