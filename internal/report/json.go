package report

import (
	"encoding/json"
	"io"

	"github.com/frankbesch/memlint/internal/lint"
)

// SchemaVersion identifies the JSON output contract. Bump it on any
// incompatible change to the shape below.
const SchemaVersion = 1

type jsonOutput struct {
	SchemaVersion int            `json:"schema_version"`
	Findings      []lint.Finding `json:"findings"`
	Summary       jsonSummary    `json:"summary"`
}

type jsonSummary struct {
	Red    int `json:"red"`
	Yellow int `json:"yellow"`
}

// JSON writes the result as a stable, never-colored JSON document. Findings
// arrive pre-sorted, so output is byte-identical across runs.
func JSON(w io.Writer, res lint.Result) error {
	findings := res.Findings
	if findings == nil {
		// Encode an empty list rather than null, so consumers can index it.
		findings = []lint.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonOutput{
		SchemaVersion: SchemaVersion,
		Findings:      findings,
		Summary:       jsonSummary{Red: res.Red(), Yellow: res.Yellow()},
	})
}
