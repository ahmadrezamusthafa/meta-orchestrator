package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"
)

// StaticContainers always samples the given containers.
func StaticContainers(ids ...string) ContainerLister {
	return func(context.Context) ([]string, error) { return ids, nil }
}

func TestDockerSamplerListsAndSumsContainers(t *testing.T) {
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch args[0] {
		case "ps":
			return []byte(`{"ID":"aaa","CreatedAt":"2026-10-17 09:30:05 +0700 WIB","Labels":"meta-orchestrator.task=TASK-1,com.docker.compose.project=app"}` + "\n" +
				`{"ID":"bbb","CreatedAt":"2026-10-17 08:00:00 +0700 WIB","Labels":""}` + "\n"), nil
		case "stats":
			if strings.Join(args[4:], ",") != "aaa,bbb" {
				t.Errorf("stats args = %v", args)
			}
			return []byte(`{"ID":"aaa","CPUPerc":"50.5%","MemUsage":"100MiB / 2GiB"}` + "\n" + `{"ID":"bbb","CPUPerc":"25%","MemUsage":"1GiB / 2GiB"}` + "\n"), nil
		}
		return nil, nil
	}
	s := NewDockerStatsSampler(runner)
	list, err := s.ListContainers(context.Background())
	if err != nil || len(list) != 2 || list[0].Labels["meta-orchestrator.task"] != "TASK-1" || list[1].Labels["meta-orchestrator.task"] != "" {
		t.Fatalf("list = %+v %v", list, err)
	}
	if want := time.Date(2026, 10, 17, 2, 30, 5, 0, time.UTC); !list[0].CreatedAt.Equal(want) {
		t.Fatalf("created = %v", list[0].CreatedAt)
	}
	mon := NewContainerMonitor(s, SpikeThresholds{})
	if err := mon.SampleOnce(context.Background(), "aaa", "bbb"); err != nil {
		t.Fatal(err)
	}
	_ = mon.SampleOnce(context.Background()) // no containers is not a sample
	sum := mon.Summary()
	if sum.Samples != 1 || sum.Containers != 2 || sum.PeakCPUPercent != 75.5 || sum.PeakMemoryMB != 1124 {
		t.Fatalf("summary = %+v", sum)
	}
}
