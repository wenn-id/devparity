package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

var immutableActionSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// splitActionRef splits "owner/repo/path@ref" into its action name and ref.
// Local (./...) and Docker (docker://...) step references have no pinnable
// ref and are reported as not an action.
func splitActionRef(uses string) (name, ref string, ok bool) {
	uses = strings.TrimSpace(uses)
	if uses == "" || strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
		return "", "", false
	}
	at := strings.LastIndex(uses, "@")
	if at <= 0 || at == len(uses)-1 {
		return uses, "", true // an action with no ref at all is still a finding
	}
	return uses[:at], uses[at+1:], true
}

// actionPins maps each third-party action to every ref it is pinned to across
// the given workflows. Steps are read from parsed YAML, so both the compact
// "- uses:" form and named steps with an indented "uses:" field are covered.
func actionPins(t *testing.T, workflows map[string]string) map[string]map[string]struct{} {
	t.Helper()
	pins := make(map[string]map[string]struct{})
	for _, steps := range allWorkflowSteps(t, workflows) {
		for _, step := range steps {
			name, ref, ok := splitActionRef(step.Uses)
			if !ok {
				continue
			}
			if pins[name] == nil {
				pins[name] = make(map[string]struct{})
			}
			pins[name][ref] = struct{}{}
		}
	}
	return pins
}

// allWorkflowSteps returns every job's steps keyed by "<workflow>/<job>".
func allWorkflowSteps(t *testing.T, workflows map[string]string) map[string][]workflowStep {
	t.Helper()
	steps := make(map[string][]workflowStep)
	for workflow, text := range workflows {
		var parsed struct {
			Jobs map[string]struct {
				Steps []workflowStep `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal([]byte(text), &parsed); err != nil {
			t.Fatalf("parse workflow %s: %v", workflow, err)
		}
		for job, jobData := range parsed.Jobs {
			steps[workflow+"/"+job] = jobData.Steps
		}
	}
	return steps
}

// assertActionsPinned requires every third-party action step in every job to
// be pinned to a full 40-hex commit SHA.
func assertActionsPinned(t *testing.T, workflows map[string]string) {
	t.Helper()
	for jobKey, steps := range allWorkflowSteps(t, workflows) {
		for _, step := range steps {
			name, ref, ok := splitActionRef(step.Uses)
			if !ok {
				continue
			}
			if !immutableActionSHA.MatchString(ref) {
				t.Fatalf("%s: action %q is not pinned to a full commit SHA: %q", jobKey, name, ref)
			}
		}
	}
}

// findActionStep returns the single step in steps using the named action.
func findActionStep(t *testing.T, steps []workflowStep, action string) workflowStep {
	t.Helper()
	var found []workflowStep
	for _, step := range steps {
		if name, _, ok := splitActionRef(step.Uses); ok && name == action {
			found = append(found, step)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one %q step, found %d", action, len(found))
	}
	return found[0]
}

// assertStepWith requires a step input to equal want.
func assertStepWith(t *testing.T, step workflowStep, key, want string) {
	t.Helper()
	got, ok := step.With[key]
	if !ok {
		t.Fatalf("step %q is missing input %q", step.Uses, key)
	}
	if fmt.Sprint(got) != want {
		t.Fatalf("step %q input %s=%v, want %q", step.Uses, key, got, want)
	}
}

// TestWorkflowPinValidationCoversNamedSteps guards the pin governance itself:
// a named step ("- name:" followed by an indented "uses:") must be visible to
// the structural parser, so an unpinned action cannot hide behind a step name.
func TestWorkflowPinValidationCoversNamedSteps(t *testing.T) {
	workflow := `jobs:
  verify:
    steps:
      - name: Named checkout
        uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803
      - name: Named setup
        uses: actions/setup-go@v5
`
	steps := allWorkflowSteps(t, map[string]string{"synthetic": workflow})["synthetic/verify"]
	if len(steps) != 2 {
		t.Fatalf("named steps=%#v, want 2 steps", steps)
	}
	name, ref, ok := splitActionRef(steps[0].Uses)
	if !ok || name != "actions/checkout" || !immutableActionSHA.MatchString(ref) {
		t.Fatalf("named step 0: name=%q ref=%q ok=%v, want SHA-pinned checkout", name, ref, ok)
	}
	name, ref, ok = splitActionRef(steps[1].Uses)
	if !ok || name != "actions/setup-go" || immutableActionSHA.MatchString(ref) {
		t.Fatalf("named step 1: name=%q ref=%q ok=%v, want mutable setup-go ref", name, ref, ok)
	}
	if _, ok := actionPins(t, map[string]string{"synthetic": workflow})["actions/setup-go"]; !ok {
		t.Fatal("actionPins ignores named steps")
	}
}

// TestSplitActionRefSkipsLocalAndDockerSteps documents that local composite
// and Docker steps carry no pinnable commit SHA and must not be flagged.
func TestSplitActionRefSkipsLocalAndDockerSteps(t *testing.T) {
	for _, uses := range []string{"./.github/workflows/verify.yml", "docker://alpine:3.20", ""} {
		if _, _, ok := splitActionRef(uses); ok {
			t.Fatalf("uses=%q was treated as a pinnable action", uses)
		}
	}
}

func TestActionHasSafeCompositeInputs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Inputs map[string]struct {
			Required bool   `yaml:"required"`
			Default  string `yaml:"default"`
		} `yaml:"inputs"`
		Runs struct {
			Using string `yaml:"using"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"version", "strict"} {
		input, ok := action.Inputs[name]
		if !ok || !input.Required || input.Default == "" {
			t.Fatalf("input %q=%#v", name, input)
		}
	}
	if action.Runs.Using != "composite" {
		t.Fatalf("using=%q", action.Runs.Using)
	}
	text := string(data)
	for _, forbidden := range []string{"pull_request_target", "contents: write", "permissions:\n  contents: write"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("action contains forbidden %q", forbidden)
		}
	}
}

func TestPublicBetaDocumentationCoversInstallActionSecurityAndGovernance(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, path...)...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	readme := read("README.md")
	security := read("SECURITY.md")

	for _, required := range []string{
		"## Installation",
		"checksums.txt",
		"sha256sum",
		"shasum -a 256",
		"Get-FileHash",
		"## GitHub Action",
		"uses: wenn-id/devparity@v0.1.0-beta.1",
		"version:",
		"strict:",
		"Linux",
		"macOS",
		"Windows",
		"## Repository governance",
		"verify / test (ubuntu-latest)",
		"verify / test (windows-latest)",
		"verify / test (macos-14)",
		"verify / container",
		"main",
	} {
		if !strings.Contains(readme, required) {
			t.Fatalf("README missing %q", required)
		}
	}
	for _, required := range []string{
		"# Security Policy",
		"https://github.com/wenn-id/devparity/security/advisories/new",
		"## Supported Versions",
		"v0.1.x",
		"Unsupported",
	} {
		if !strings.Contains(security, required) {
			t.Fatalf("SECURITY.md missing %q", required)
		}
	}
}

func TestActionUsesPortableWorkspaceSafeDownloadSteps(t *testing.T) {
	read := func(path ...string) string {
		parts := append([]string{"..", ".."}, path...)
		data, err := os.ReadFile(filepath.Join(parts...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	actionText := read("action.yml")
	entrypointText := read("scripts", "action-entrypoint.sh")
	windowsEntrypointText := read("scripts", "action-entrypoint.ps1")
	if !strings.Contains(actionText, `bash "$GITHUB_ACTION_PATH/scripts/action-entrypoint.sh"`) {
		t.Fatal("Unix composite step does not delegate to the tested action entrypoint")
	}
	if !strings.Contains(actionText, `& "$env:GITHUB_ACTION_PATH\scripts\action-entrypoint.ps1"`) {
		t.Fatal("Windows composite step does not delegate to the extracted PowerShell entrypoint")
	}
	if strings.Contains(actionText, "DEVPARITY_RELEASE_BASE") {
		t.Fatal("composite action must not expose a release-base environment override")
	}
	if !strings.Contains(actionText, "run: |\n        bash \"$GITHUB_ACTION_PATH/scripts/action-entrypoint.sh\"\n") {
		t.Fatal("Unix composite step must invoke the entrypoint without positional arguments")
	}
	for _, required := range []string{
		"runner.os != 'Windows'",
		"runner.os == 'Windows'",
		"shell: powershell",
	} {
		if !strings.Contains(actionText, required) {
			t.Fatalf("action missing Windows portable behavior %q", required)
		}
	}
	for _, required := range []string{
		"ReleaseBaseUrl",
		"Windows-X64",
		"devparity-windows-amd64.exe",
		"Invoke-WebRequest -UseBasicParsing -Uri",
		"Get-FileHash -Algorithm SHA256",
		"Join-Path $env:RUNNER_TEMP",
		"finally {",
		"Remove-Item",
		"doctor --format github",
	} {
		if !strings.Contains(windowsEntrypointText, required) {
			t.Fatalf("Windows action entrypoint missing portable behavior %q", required)
		}
	}
	for _, required := range []string{
		"Linux-X64) asset=devparity-linux-amd64",
		"Linux-ARM64) asset=devparity-linux-arm64",
		"macOS-X64) asset=devparity-darwin-amd64",
		"macOS-ARM64) asset=devparity-darwin-arm64",
		"RUNNER_TEMP",
		"mktemp -d",
		"trap 'rm -rf",
		"--output \"$workdir/$asset\"",
		"--output \"$workdir/checksums.txt\"",
		`^[0-9a-fA-F]{64}  ${asset}$`,
		"GITHUB_WORKSPACE",
		"doctor --format github",
	} {
		if !strings.Contains(entrypointText, required) {
			t.Fatalf("action entrypoint missing portable workspace-safe behavior %q", required)
		}
	}
	for _, forbidden := range []string{"curl -fsSLO", "actions/checkout", "contents: write"} {
		if strings.Contains(actionText+entrypointText, forbidden) {
			t.Fatalf("action still relies on forbidden behavior %q", forbidden)
		}
	}
	if strings.Contains(entrypointText, "DEVPARITY_RELEASE_BASE") {
		t.Fatal("shipped action entrypoint must not accept a release-base environment override")
	}
	if !strings.Contains(entrypointText, `base="${1:-https://github.com/wenn-id/devparity/releases/download/${DEVPARITY_VERSION}}"`) {
		t.Fatal("action entrypoint must derive its production base from the fixed release URL")
	}
	if !strings.Contains(read("scripts", "release-smoke.sh"), `bash "$ROOT/scripts/action-entrypoint.sh" "file://$DIST"`) {
		t.Fatal("release smoke must pass its local fixture base explicitly")
	}
}

func TestActionChecksumWorksOnMacOS(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "action-entrypoint.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.Contains(text, "shasum -a 256") {
		t.Fatal("action entrypoint has no macOS-compatible checksum fallback")
	}
	if !strings.Contains(text, "sha256sum") {
		t.Fatal("action entrypoint lost the Linux sha256sum path")
	}
}

func TestReleaseActionSmoke(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("release smoke uses Bash and GNU checksum tooling")
	}
	if os.Getenv("DEVPARITY_RELEASE_SMOKE") != "1" {
		t.Skip("set DEVPARITY_RELEASE_SMOKE=1 to run release smoke")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release-smoke.sh"))
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release smoke failed: %v\n%s", err, output)
	}
}

func TestReleaseWorkflowUsesReadOnlyBuildJobsAndPinnedActions(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	ciText := read("..", "..", ".github", "workflows", "ci.yml")
	verifyText := read("..", "..", ".github", "workflows", "verify.yml")
	releaseText := read("..", "..", ".github", "workflows", "release.yml")

	workflows := map[string]string{
		"ci":      ciText,
		"verify":  verifyText,
		"release": releaseText,
	}
	assertActionsPinned(t, workflows)
	// Actions are pinned by immutable commit SHA, not by version. The exact
	// SHA is intentionally not hardcoded here: Dependabot bumps it, and a
	// test that pins the digest would fail every dependency update while
	// proving nothing beyond "the digest is the one I typed". What matters
	// is that each workflow still uses the actions it needs, that they are
	// pinned to a 40-hex SHA, and that the pin is consistent repo-wide.
	for workflow, required := range map[string][]string{
		"verify":  {"actions/checkout", "actions/setup-go"},
		"release": {"actions/checkout", "actions/setup-go", "actions/upload-artifact", "actions/download-artifact"},
	} {
		present := actionPins(t, map[string]string{workflow: workflows[workflow]})
		for _, action := range required {
			if _, ok := present[action]; !ok {
				t.Fatalf("workflow %s is missing pinned action %q", workflow, action)
			}
		}
	}
	pins := actionPins(t, workflows)
	for action, refs := range pins {
		if len(refs) != 1 {
			t.Fatalf("action %q is pinned to %d different SHAs %v; keep one pin per action", action, len(refs), refs)
		}
	}
	for _, mutable := range []string{"actions/checkout@v", "actions/setup-go@v", "actions/upload-artifact@v", "actions/download-artifact@v"} {
		for workflow, text := range workflows {
			if strings.Contains(text, mutable) {
				t.Fatalf("workflow %s still uses mutable action reference %q", workflow, mutable)
			}
		}
	}
	if strings.Count(verifyText, "persist-credentials: false") != 3 ||
		strings.Count(releaseText, "persist-credentials: false") != 1 {
		t.Fatal("all non-publish checkouts must disable persisted credentials")
	}
	for _, workflow := range []string{ciText, verifyText, releaseText} {
		if strings.Contains(workflow, "contents: write") && !strings.Contains(workflow, "  publish:") {
			t.Fatal("non-publish workflow requests write permission")
		}
	}
	if !strings.Contains(releaseText, "  publish:\n    needs: [verify, package]") ||
		!strings.Contains(releaseText, "  publish:\n    needs: [verify, package]\n    runs-on: ubuntu-latest\n    permissions:\n      contents: write") {
		t.Fatal("publish job does not isolate contents: write")
	}
	if strings.Contains(verifyText, "contents: write") || strings.Contains(ciText, "contents: write") {
		t.Fatal("CI/verify workflow is not read-only")
	}
	if !strings.Contains(verifyText, "if: runner.os == 'Linux'\n        run: go mod tidy -diff") {
		t.Fatal("verify workflow does not enforce a Linux go mod tidy -diff gate")
	}
	if !strings.Contains(verifyText, "name: Install staticcheck\n        if: runner.os == 'Linux'\n        shell: bash") {
		t.Fatal("staticcheck installation must use the Linux quality gate")
	}
	testJobSteps := workflowJobSteps(t, verifyText, "test")
	if !containsWorkflowRun(testJobSteps, "go mod tidy -diff", "runner.os == 'Linux'") {
		t.Fatal("verify test job does not execute go mod tidy -diff")
	}
	if !containsWorkflowRun(testJobSteps, "staticcheck ./...", "runner.os == 'Linux'") {
		t.Fatal("verify test job does not execute staticcheck")
	}
}

func TestRepoGovernanceAndSecurityScanning(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			return "" // a removed/absent file must fail the contains check below
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	root := filepath.Join("..", "..")
	verifyData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "verify.yml"))
	if err != nil {
		t.Fatal(err)
	}
	scanText := read(root, ".github", "workflows", "codeql.yml")
	dependabotData, err := os.ReadFile(filepath.Join(root, ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ownersText := read(root, ".github", "CODEOWNERS")
	contribText := read(root, "CONTRIBUTING.md")
	cocText := read(root, "CODE_OF_CONDUCT.md")

	verifyText := string(verifyData)
	// Gosec must run install + scan as Linux-only steps in the verify job.
	if !strings.Contains(verifyText, "go install github.com/securego/gosec/v2/cmd/gosec@v2.22.8") {
		t.Fatal("verify workflow missing gosec install")
	}
	linuxSteps := workflowJobSteps(t, verifyText, "test")
	if !containsWorkflowRun(linuxSteps, "gosec -quiet ./...", "runner.os == 'Linux'") {
		t.Fatal("verify job has no Linux-gated gosec scan step")
	}

	// CodeQL: parse the analyze job and assert scoped permission + ordered pinned action steps.
	var codeql struct {
		Jobs map[string]struct {
			Permissions map[string]string `yaml:"permissions"`
			Steps       []workflowStep    `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(scanText), &codeql); err != nil {
		t.Fatalf("parse codeql workflow: %v", err)
	}
	analyze, ok := codeql.Jobs["analyze"]
	if !ok {
		t.Fatal("codeql workflow missing analyze job")
	}
	if analyze.Permissions["security-events"] != "write" {
		t.Fatal("codeql analyze job must scope security-events: write")
	}
	const codeqlSHA = "9ee088e13615f8d1eaef4766f9dde95d3356a8f6"
	codeqlRuns := []string{}
	for _, s := range analyze.Steps {
		if strings.HasPrefix(s.Uses, "github/codeql-action/") {
			codeqlRuns = append(codeqlRuns, s.Uses)
		}
	}
	want := []string{
		"github/codeql-action/init@" + codeqlSHA,
		"github/codeql-action/autobuild@" + codeqlSHA,
		"github/codeql-action/analyze@" + codeqlSHA,
	}
	for i := range want {
		if i >= len(codeqlRuns) || !strings.Contains(codeqlRuns[i], want[i]) {
			t.Fatalf("codeql analyze steps must run init->autobuild->analyze pinned at %s", codeqlSHA)
		}
	}

	// Dependabot: root Go modules and GitHub Actions both on a weekly schedule.
	var dependabot struct {
		Updates []struct {
			Ecosystem string `yaml:"package-ecosystem"`
			Directory string `yaml:"directory"`
			Schedule  struct {
				Interval string `yaml:"interval"`
			} `yaml:"schedule"`
		} `yaml:"updates"`
	}
	if err := yaml.Unmarshal(dependabotData, &dependabot); err != nil {
		t.Fatalf("parse dependabot config: %v", err)
	}
	gotSchedule := map[string]string{}
	for _, u := range dependabot.Updates {
		gotSchedule[u.Ecosystem+u.Directory] = u.Schedule.Interval
	}
	for _, wantK := range []string{"gomod/", "github-actions/"} {
		if gotSchedule[wantK] != "weekly" {
			t.Fatalf("dependabot update %q must be weekly, got %q", wantK, gotSchedule[wantK])
		}
	}

	for _, required := range []string{"@wenn-id", "SECURITY.md", "action.yml"} {
		if !strings.Contains(ownersText, required) {
			t.Fatalf("CODEOWNERS missing %q", required)
		}
	}
	for _, required := range []string{"pull request", "static", "--trust-repository", "SECURITY.md"} {
		if !strings.Contains(contribText, required) {
			t.Fatalf("CONTRIBUTING.md missing %q", required)
		}
	}
	for _, required := range []string{"Contributor Covenant", "maintainers", "harassment"} {
		if !strings.Contains(cocText, required) {
			t.Fatalf("CODE_OF_CONDUCT.md missing %q", required)
		}
	}
}

func TestReleaseSupplyChainHardening(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	releaseText := read("..", "..", ".github", "workflows", "release.yml")
	buildText := read("..", "..", "scripts", "release-build.sh")
	entrypointText := read("..", "..", "scripts", "action-entrypoint.sh")
	windowsEntrypointText := read("..", "..", "scripts", "action-entrypoint.ps1")

	for _, required := range []string{
		"id-token: write",
		"attestations: write",
		"actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8",
		"anchore/sbom-action@f8bdd1d8ac5e901a77a92f111440fdb1b593736b",
		"output-file: dist/devparity.spdx.json",
		"run: (cd dist && sha256sum -c checksums.txt)",
	} {
		if !strings.Contains(releaseText, required) {
			t.Fatalf("release workflow missing %q", required)
		}
	}
	if !strings.Contains(buildText, "go mod verify") {
		t.Fatal("release builder does not verify downloaded modules")
	}
	for name, text := range map[string]string{"Unix action": entrypointText, "Windows action": windowsEntrypointText} {
		if !strings.Contains(text, "gh attestation verify") || !strings.Contains(text, "--repo wenn-id/devparity") {
			t.Fatalf("%s does not verify GitHub provenance", name)
		}
	}
}

type workflowStep struct {
	Run  string                 `yaml:"run"`
	If   string                 `yaml:"if"`
	Uses string                 `yaml:"uses"`
	With map[string]interface{} `yaml:"with"`
}

func workflowJobSteps(t *testing.T, workflow, job string) []workflowStep {
	t.Helper()
	var parsed struct {
		Jobs map[string]struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &parsed); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	jobData, ok := parsed.Jobs[job]
	if !ok {
		t.Fatalf("workflow missing job %q", job)
	}
	return jobData.Steps
}

func containsWorkflowRun(steps []workflowStep, command, condition string) bool {
	for _, step := range steps {
		if strings.TrimSpace(step.Run) == command && strings.TrimSpace(step.If) == condition {
			return true
		}
	}
	return false
}

func TestReleaseEmbedsVersionAndVerifiesEveryAsset(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	releaseText := read("..", "..", ".github", "workflows", "release.yml")
	buildText := read("..", "..", "scripts", "release-build.sh")

	if !strings.Contains(releaseText, `VERSION="$VERSION" DEST=dist bash scripts/release-build.sh`) {
		t.Fatal("release workflow does not use the shared release builder")
	}
	text := buildText
	const linkerTarget = "-X github.com/wenn-id/devparity/internal/cli.Version=${VERSION}"
	if !strings.Contains(text, linkerTarget) {
		t.Fatalf("release builder missing linker target %q", linkerTarget)
	}
	if strings.Contains(releaseText, "-X github.com/devparity/devparity/internal/cli.Version=") || strings.Contains(text, "-X github.com/devparity/devparity/internal/cli.Version=") {
		t.Fatal("release builder still uses the old module path for Version")
	}
	for _, required := range []string{
		`for target in \`,
		"grep -a -F -- \"$VERSION\" \"${DEST}/${asset}\" >/dev/null",
		`native_os="$(go env GOOS)"`,
		`native_arch="$(go env GOARCH)"`,
		`native_asset="devparity-${native_os}-${native_arch}"`,
		`test "$("${DEST}/${native_asset}" version)" = "$VERSION"`,
		`printf '%s  %s\n' "$(sha256sum "$DEST/$asset" | awk '{print $1}')" "$asset"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("release builder missing %q", required)
		}
	}
	for _, asset := range []string{
		"devparity-linux-amd64",
		"devparity-linux-arm64",
		"devparity-darwin-amd64",
		"devparity-darwin-arm64",
		"devparity-windows-amd64.exe",
	} {
		if !strings.Contains(text, "asset="+asset) {
			t.Fatalf("release builder does not generate asset %q", asset)
		}
	}
}

func TestReleaseSmokeUsesAdvertisedDefaultVersion(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	actionText := read("..", "..", "action.yml")
	smokeText := read("..", "..", "scripts", "release-smoke.sh")
	const defaultVersion = "v0.1.0-beta.1"
	if !strings.Contains(actionText, "default: "+defaultVersion) {
		t.Fatalf("action does not advertise default version %q", defaultVersion)
	}
	if !strings.Contains(smokeText, `VERSION="${DEVPARITY_SMOKE_VERSION:-`+defaultVersion+`}"`) {
		t.Fatalf("release smoke does not exercise advertised default version %q", defaultVersion)
	}
	if !strings.Contains(smokeText, "scripts/action-entrypoint.sh") {
		t.Fatal("release smoke does not execute the composite action entrypoint")
	}
}

func TestReleaseChecksumsUseBasenames(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	releaseText := read("..", "..", ".github", "workflows", "release.yml")
	buildText := read("..", "..", "scripts", "release-build.sh")
	entrypointText := read("..", "..", "scripts", "action-entrypoint.sh")
	smokeText := read("..", "..", "scripts", "release-smoke.sh")
	verifyText := read("..", "..", ".github", "workflows", "verify.yml")

	const manifestCommand = `printf '%s  %s\n' "$(sha256sum "$DEST/$asset" | awk '{print $1}')" "$asset"`
	const bashVerifyPattern = `^[0-9a-fA-F]{64}  ${asset}$`
	if !strings.Contains(buildText, manifestCommand) {
		t.Fatalf("release builder does not generate basename checksums with %q", manifestCommand)
	}
	if !strings.Contains(entrypointText, bashVerifyPattern) {
		t.Fatal("action entrypoint checksum verification no longer expects basename entries")
	}
	ps1Text := read("..", "..", "scripts", "action-entrypoint.ps1")
	if !strings.Contains(ps1Text, `$parts = $line.Split([char[]]' ', [System.StringSplitOptions]::RemoveEmptyEntries)`) ||
		!strings.Contains(ps1Text, `if ($parts.Count -ge 2 -and $parts[1] -eq $asset)`) {
		t.Fatal("Windows action entrypoint checksum verification no longer matches the shared basename contract")
	}
	if strings.Contains(releaseText+buildText, "sha256sum dist/* > dist/checksums.txt") {
		t.Fatal("release workflow still writes dist-prefixed checksum paths")
	}
	const smokeStepName = "        name: End-to-end release and composite-action smoke"
	stepStart := strings.Index(verifyText, smokeStepName)
	if stepStart == -1 {
		t.Fatal("CI is missing the release checksum smoke test")
	}
	runStart := strings.Index(verifyText[stepStart:], "        run: |\n")
	if runStart == -1 {
		t.Fatal("CI checksum smoke test is missing its run block")
	}
	runLines := make([]string, 0)
	for _, line := range strings.Split(verifyText[stepStart+runStart+len("        run: |\n"):], "\n") {
		if line != "" && len(line)-len(strings.TrimLeft(line, " ")) < 10 {
			break
		}
		runLines = append(runLines, line)
	}
	smokeRun := strings.Join(runLines, "\n")
	if !strings.Contains(smokeRun, "scripts/release-smoke.sh") {
		t.Fatal("CI smoke test does not execute the end-to-end release smoke script")
	}
	for _, asset := range []string{
		"devparity-linux-amd64",
		"devparity-linux-arm64",
		"devparity-darwin-amd64",
		"devparity-darwin-arm64",
		"devparity-windows-amd64.exe",
	} {
		if !strings.Contains(smokeText, asset) {
			t.Fatalf("release smoke does not cover asset %q", asset)
		}
	}
}

func TestReleaseWaitsForAllVerificationGates(t *testing.T) {
	read := func(path ...string) string {
		data, err := os.ReadFile(filepath.Join(path...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "\r\n", "\n")
	}
	verifyText := read("..", "..", ".github", "workflows", "verify.yml")
	ciText := read("..", "..", ".github", "workflows", "ci.yml")
	releaseText := read("..", "..", ".github", "workflows", "release.yml")
	extractJob := func(text, job string) string {
		lines := strings.Split(text, "\n")
		start := -1
		for i, line := range lines {
			if line == "  "+job+":" {
				start = i
				break
			}
		}
		if start == -1 {
			t.Fatalf("workflow is missing job %q", job)
		}
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "  ") && !strings.HasPrefix(lines[i], "    ") && strings.TrimSpace(lines[i]) != "" {
				end = i
				break
			}
		}
		return strings.Join(lines[start:end], "\n")
	}

	ciVerifyJob := extractJob(ciText, "verify")
	releaseVerifyJob := extractJob(releaseText, "verify")
	packageJob := extractJob(releaseText, "package")
	publishJob := extractJob(releaseText, "publish")

	if !strings.Contains(verifyText, "workflow_call:") {
		t.Fatal("verify workflow is not reusable")
	}
	if !strings.Contains(verifyText, "permissions:\n  contents: read") {
		t.Fatal("verification workflow does not declare read-only permissions")
	}
	for _, forbidden := range []string{"contents: write", "write-all", "permissions: write"} {
		if strings.Contains(verifyText, forbidden) {
			t.Fatalf("verification workflow requests write-capable permission %q", forbidden)
		}
	}
	if strings.Count(verifyText, "ref: ${{ github.sha }}") < 3 {
		t.Fatal("verification workflow does not check out the caller commit")
	}
	for _, gate := range []string{
		"gofmt -w .",
		"go vet ./...",
		"go test ./...",
		"go test -race ./...",
		"go build -trimpath ./cmd/devparity",
		"End-to-end release and composite-action smoke",
		"DEVPARITY_CONTAINER_TEST: \"1\"",
		"go test -v ./...",
	} {
		if !strings.Contains(verifyText, gate) {
			t.Fatalf("verification workflow missing %q", gate)
		}
	}
	if !strings.Contains(ciText, "push:") || !strings.Contains(ciText, "pull_request:") {
		t.Fatal("CI does not trigger for push and pull-request events")
	}
	if !strings.Contains(ciVerifyJob, "uses: ./.github/workflows/verify.yml") {
		t.Fatal("CI does not call reusable verification workflow")
	}
	if !strings.Contains(ciVerifyJob, "permissions:\n      contents: read") {
		t.Fatal("CI verification caller does not use read-only permissions")
	}
	for _, forbidden := range []string{"contents: write", "write-all"} {
		if strings.Contains(ciVerifyJob, forbidden) {
			t.Fatalf("CI verification caller requests write-capable permission %q", forbidden)
		}
	}
	if !strings.Contains(releaseVerifyJob, "uses: ./.github/workflows/verify.yml") {
		t.Fatal("release does not call reusable verification workflow")
	}
	if !strings.Contains(releaseVerifyJob, "permissions:\n      contents: read") {
		t.Fatal("release verification caller does not use read-only permissions")
	}
	for _, forbidden := range []string{"contents: write", "write-all"} {
		if strings.Contains(releaseVerifyJob, forbidden) {
			t.Fatalf("release verification caller requests write-capable permission %q", forbidden)
		}
	}
	if !strings.Contains(packageJob, "needs: verify") {
		t.Fatal("package job does not wait for verification")
	}
	if !strings.Contains(packageJob, "permissions:\n      contents: read") {
		t.Fatal("package job does not use read-only permissions")
	}
	if strings.Contains(packageJob, "contents: write") || strings.Contains(packageJob, "write-all") {
		t.Fatal("package job requests write-capable permissions")
	}
	if !strings.Contains(packageJob, "ref: ${{ github.sha }}") {
		t.Fatal("release package job does not check out the caller commit")
	}
	releaseSteps := workflowJobSteps(t, releaseText, "package")
	uploadStep := findActionStep(t, releaseSteps, "actions/upload-artifact")
	assertStepWith(t, uploadStep, "name", "release-assets")
	assertStepWith(t, uploadStep, "path", "dist")
	if !strings.Contains(publishJob, "needs: [verify, package]") {
		t.Fatal("publish does not wait for verification and packaging")
	}
	if !strings.Contains(publishJob, "permissions:\n      contents: write") {
		t.Fatal("publish job lacks isolated write permission")
	}
	publishSteps := workflowJobSteps(t, releaseText, "publish")
	downloadStep := findActionStep(t, publishSteps, "actions/download-artifact")
	assertStepWith(t, downloadStep, "name", "release-assets")
	assertStepWith(t, downloadStep, "path", "dist")
	for _, forbidden := range []string{"actions/checkout@", "actions/setup-go@", "gofmt", "go test", "go build"} {
		if strings.Contains(publishJob, forbidden) {
			t.Fatalf("publish job must not contain %q", forbidden)
		}
	}
	if !strings.Contains(publishJob, "gh release create") {
		t.Fatal("publish job does not create the release")
	}
}
