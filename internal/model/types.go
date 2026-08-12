package model

type Status string
type Severity string
type FactKind string

const (
	StatusPass         Status   = "pass"
	StatusFinding      Status   = "finding"
	StatusSkipped      Status   = "skipped"
	StatusInconclusive Status   = "inconclusive"
	SeverityInfo       Severity = "info"
	SeverityWarning    Severity = "warning"
	SeverityError      Severity = "error"
)

type SourceRef struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Field string `json:"field"`
}

type Fact struct {
	Kind    FactKind  `json:"kind"`
	Subject string    `json:"subject"`
	Value   string    `json:"value"`
	Source  SourceRef `json:"source"`
}

type Finding struct {
	RuleID     string   `json:"rule_id"`
	Severity   Severity `json:"severity"`
	Status     Status   `json:"status"`
	Message    string   `json:"message"`
	Evidence   []Fact   `json:"evidence"`
	Suggestion string   `json:"suggestion"`
}

type DocBlock struct {
	ID     string    `json:"id"`
	Shell  string    `json:"shell"`
	Script string    `json:"script"`
	Source SourceRef `json:"source"`
}

type ExecutionResult struct {
	BlockID  string `json:"block_id"`
	Mode     string `json:"mode"`
	ExitCode int    `json:"exit_code"`
	Duration int64  `json:"duration_ms"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Status   Status `json:"status"`
}

type Summary struct {
	Pass         int `json:"pass"`
	Finding      int `json:"finding"`
	Skipped      int `json:"skipped"`
	Inconclusive int `json:"inconclusive"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	ToolVersion   string    `json:"tool_version"`
	Repository    string    `json:"repository"`
	Summary       Summary   `json:"summary"`
	Results       []Finding `json:"results"`
}
