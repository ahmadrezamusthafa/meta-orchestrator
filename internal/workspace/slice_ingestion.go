package workspace

import (
	"path/filepath"
	"sort"
	"strings"
)

// A task can start mid-pipeline (a slice). Documents the operator uploads with it — PRD.md,
// TECH_DOC.md, TASK_PLAN.md — stand in for the output of the stages the slice skips, so later
// stages, evidence and pull requests read them exactly as if those stages had run.

// sliceDocStages maps an uploaded document's file name (case-insensitive) to the stage whose
// output it replaces.
var sliceDocStages = map[string]string{
	"prd.md":          "prd_discovery",
	"tech_doc.md":     "techdoc_rfc",
	"tech_doc_rfc.md": "techdoc_rfc",
	"techdoc.md":      "techdoc_rfc",
	"rfc.md":          "techdoc_rfc",
	"task_plan.md":    "task_breakdown",
}

// SliceDocument is an uploaded document that becomes a skipped stage's output.
type SliceDocument struct {
	Stage   string
	Name    string
	Content string
}

// StageForDocument returns the stage an uploaded file name stands in for.
func StageForDocument(name string) (string, bool) {
	stage, ok := sliceDocStages[strings.ToLower(filepath.Base(name))]
	return stage, ok
}

// PlanSliceIngestion decides which uploaded documents become stage outputs: those that map to a
// stage before startStage in pipeline. The rest are returned in ignored with the reason, sorted
// by name. Empty documents are ignored.
func PlanSliceIngestion(docs map[string]string, pipeline []string, startStage string) (ingest []SliceDocument, ignored []string) {
	index := map[string]int{}
	for i, st := range pipeline {
		index[st] = i
	}
	start, ok := index[startStage]
	if !ok {
		start = 0
	}
	names := make([]string, 0, len(docs))
	for n := range docs {
		names = append(names, n)
	}
	sort.Strings(names)
	taken := map[string]string{}
	for _, name := range names {
		content := docs[name]
		stage, known := StageForDocument(name)
		i, inPipeline := index[stage]
		switch {
		case strings.TrimSpace(content) == "":
			ignored = append(ignored, name+": empty")
		case !known || !inPipeline:
			ignored = append(ignored, name+": not a PRD.md, TECH_DOC.md or TASK_PLAN.md")
		case i >= start:
			ignored = append(ignored, name+": the task runs "+stage+" itself")
		case taken[stage] != "":
			ignored = append(ignored, name+": "+taken[stage]+" already provides "+stage)
		default:
			taken[stage] = name
			ingest = append(ingest, SliceDocument{Stage: stage, Name: filepath.Base(name), Content: content})
		}
	}
	sort.SliceStable(ingest, func(a, b int) bool { return index[ingest[a].Stage] < index[ingest[b].Stage] })
	return ingest, ignored
}
