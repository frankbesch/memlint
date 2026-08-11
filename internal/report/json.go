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
	SchemaVersion int           `json:"schema_version"`
	Findings      []jsonFinding `json:"findings"`
	Summary       jsonSummary   `json:"summary"`
}

// jsonFinding decorates a finding with its documentation link. doc_url is
// derived at render time rather than stored on the finding: it is a property
// of the report, not of the invariant.
type jsonFinding struct {
	lint.Finding
	DocURL string `json:"doc_url"`
}

type jsonSummary struct {
	Red    int `json:"red"`
	Yellow int `json:"yellow"`
}

// JSON writes the result as a stable, never-colored JSON document. Findings
// arrive pre-sorted, so output is byte-identical across runs.
func JSON(w io.Writer, res lint.Result) error {
	// Build an empty list rather than null, so consumers can index it.
	findings := make([]jsonFinding, 0, len(res.Findings))
	for _, f := range res.Findings {
		findings = append(findings, jsonFinding{Finding: f, DocURL: DocURL(f.Code)})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonOutput{
		SchemaVersion: SchemaVersion,
		Findings:      findings,
		Summary:       jsonSummary{Red: res.Red(), Yellow: res.Yellow()},
	})
}
