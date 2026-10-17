package skills

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestExecutorRefusesSkillsWithNothingToRun(t *testing.T) {
	e := NewStandardSkillExecutor()
	for _, f := range []types.SkillFormat{types.SkillFormatMCP, types.SkillFormatNative, types.SkillFormatBMAD} {
		_, err := e.Execute(context.Background(), &types.UniversalSkillContract{Name: "x", SourceFormat: f}, &types.SkillExecutionRequest{}, nil)
		if !errors.Is(err, ErrNotRunnable) {
			t.Errorf("%s without a command: err = %v, want ErrNotRunnable", f, err)
		}
	}
}

func TestExecutorCapsOutputAndTimesOut(t *testing.T) {
	e := &StandardSkillExecutor{MaxOutputBytes: 10}
	res, err := e.Execute(context.Background(), &types.UniversalSkillContract{Name: "big", SourceFormat: types.SkillFormatMCP,
		Command: "sh", Args: []string{"-c", "printf 0123456789abcdefghij"}}, &types.SkillExecutionRequest{}, nil)
	if err != nil || !strings.HasPrefix(res.Stdout, "0123456789\n[… 10 more bytes") {
		t.Fatalf("res = %+v, err = %v", res, err)
	}

	_, err = e.Execute(context.Background(), &types.UniversalSkillContract{Name: "slow", SourceFormat: types.SkillFormatMCP,
		Command: "sleep", Args: []string{"5"}, TimeoutSeconds: 1}, &types.SkillExecutionRequest{}, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}
