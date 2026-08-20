package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

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
	if !strings.Contains(actionText, `bash "$GITHUB_ACTION_PATH/scripts/action-entrypoint.sh"`) {
		t.Fatal("Unix composite step does not delegate to the tested action entrypoint")
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
		"Invoke-WebRequest",
		"Get-FileHash -Algorithm SHA256",
		"Join-Path $env:RUNNER_TEMP",
		"finally {",
		"Remove-Item",
	} {
		if !strings.Contains(actionText, required) {
			t.Fatalf("action missing Windows portable behavior %q", required)
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
		"grep \"  ${asset}$\" checksums.txt | sha256sum -c -",
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

	const (
		checkout = "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"
		setupGo  = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
		upload   = "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a"
		download = "actions/download-artifact@37930b1c2abaa49bbe596cd826c3c89aef350131"
	)
	allText := ciText + verifyText + releaseText
	for _, action := range []string{checkout, setupGo, upload, download} {
		if !strings.Contains(allText, action) {
			t.Fatalf("workflow files missing pinned action %q", action)
		}
	}
	for _, mutable := range []string{"actions/checkout@v", "actions/setup-go@v", "actions/upload-artifact@v", "actions/download-artifact@v"} {
		if strings.Contains(allText, mutable) {
			t.Fatalf("workflow files still use mutable action reference %q", mutable)
		}
	}
	if strings.Count(verifyText, "persist-credentials: false") != 2 ||
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

type workflowStep struct {
	Run string `yaml:"run"`
	If  string `yaml:"if"`
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
		`test "$("${DEST}/devparity-linux-amd64" version)" = "$VERSION"`,
		`(cd "$DEST" && sha256sum "${assets[@]}" > checksums.txt)`,
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

	const manifestCommand = `(cd "$DEST" && sha256sum "${assets[@]}" > checksums.txt)`
	const verifyCommand = `grep "  ${asset}$" checksums.txt | sha256sum -c -`
	if !strings.Contains(buildText, manifestCommand) {
		t.Fatalf("release builder does not generate basename checksums with %q", manifestCommand)
	}
	if !strings.Contains(entrypointText, verifyCommand) {
		t.Fatal("action entrypoint checksum verification no longer expects basename entries")
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
	if strings.Count(verifyText, "ref: ${{ github.sha }}") < 2 {
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
	if !strings.Contains(packageJob, "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a") || !strings.Contains(packageJob, "name: release-assets") {
		t.Fatal("package job does not upload the release-assets artifact")
	}
	if !strings.Contains(publishJob, "needs: [verify, package]") {
		t.Fatal("publish does not wait for verification and packaging")
	}
	if !strings.Contains(publishJob, "permissions:\n      contents: write") {
		t.Fatal("publish job lacks isolated write permission")
	}
	if !strings.Contains(publishJob, "actions/download-artifact@37930b1c2abaa49bbe596cd826c3c89aef350131") || strings.Count(publishJob, "name: release-assets") != 1 {
		t.Fatal("publish job does not download exactly the release-assets artifact")
	}
	for _, forbidden := range []string{"actions/checkout@", "actions/setup-go@", "gofmt", "go test", "go build"} {
		if strings.Contains(publishJob, forbidden) {
			t.Fatalf("publish job must not contain %q", forbidden)
		}
	}
	if !strings.Contains(publishJob, "gh release create") {
		t.Fatal("publish job does not create the release")
	}
}
