package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/frankbesch/memlint/internal/lint"
)

// The summary gains an additive "info" count; red and yellow are unchanged
// and schema_version stays 1.
func TestJSONSummaryCountsInfo(t *testing.T) {
	res := lint.Result{RulesRun: 1, Findings: []lint.Finding{
		{Rule: "append_only", Code: "append_only/rotated", Severity: lint.SeverityInfo, Path: "a.md", Message: "rotated"},
		{Rule: "junk", Code: "junk/match", Severity: lint.SeverityYellow, Path: "b.tmp", Message: "junk"},
	}}
	var b strings.Builder
	if err := JSON(&b, res); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SchemaVersion int `json:"schema_version"`
		Findings      []struct {
			Severity string `json:"severity"`
		} `json:"findings"`
		Summary struct{ Red, Yellow, Info int }
	}
	if err := json.Unmarshal([]byte(b.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != 1 {
		t.Errorf("schema_version must stay 1, got %d", doc.SchemaVersion)
	}
	if doc.Summary.Red != 0 || doc.Summary.Yellow != 1 || doc.Summary.Info != 1 {
		t.Errorf("summary got %+v, want 0/1/1", doc.Summary)
	}
	if doc.Findings[0].Severity != "INFO" {
		t.Errorf("INFO severity must serialize as INFO, got %q", doc.Findings[0].Severity)
	}
}
