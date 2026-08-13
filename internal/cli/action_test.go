package cli

import (
	"os"
	"path/filepath"
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

func TestReleaseEmbedsVersionAndVerifiesEveryAsset(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	const linkerTarget = "-X github.com/wenn-id/devparity/internal/cli.Version=${VERSION}"
	if !strings.Contains(text, linkerTarget) {
		t.Fatalf("release workflow missing linker target %q", linkerTarget)
	}
	if strings.Contains(text, "-X github.com/devparity/devparity/internal/cli.Version=") {
		t.Fatal("release workflow still uses the old module path for Version")
	}
	const buildLoopHeader = "for target in \\\n"
	buildLoopStart := strings.Index(text, buildLoopHeader)
	if buildLoopStart == -1 {
		t.Fatal("release workflow does not define the release asset build loop")
	}
	buildLoopEnd := strings.Index(text[buildLoopStart:], "\n          done")
	if buildLoopEnd == -1 {
		t.Fatal("release workflow release asset build loop is incomplete")
	}
	buildLoop := text[buildLoopStart : buildLoopStart+buildLoopEnd]
	if !strings.Contains(text, "assets=(") {
		t.Fatal("release workflow does not collect release assets for validation")
	}
	for _, asset := range []string{
		"devparity-linux-amd64",
		"devparity-linux-arm64",
		"devparity-darwin-amd64",
		"devparity-darwin-arm64",
		"devparity-windows-amd64.exe",
	} {
		if !strings.Contains(buildLoop, "asset="+asset) {
			t.Fatalf("release workflow build loop does not generate asset %q", asset)
		}
	}
	if !strings.Contains(buildLoop, `assets+=("$asset")`) {
		t.Fatal("release workflow does not append each built asset for validation")
	}
	if !strings.Contains(text, `for asset in "${assets[@]}"; do`) {
		t.Fatal("release workflow does not verify every release asset")
	}
	if !strings.Contains(text, `grep -a -F -- "$VERSION" "dist/$asset" >/dev/null`) {
		t.Fatal("release workflow does not verify each asset's embedded version")
	}
	if !strings.Contains(text, `test "$(./dist/devparity-linux-amd64 version)" = "$VERSION"`) {
		t.Fatal("release workflow does not execute a release asset to verify its version")
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
	actionText := read("..", "..", "action.yml")
	verifyText := read("..", "..", ".github", "workflows", "verify.yml")

	const manifestCommand = `(cd dist && sha256sum "${assets[@]}" > checksums.txt)`
	if !strings.Contains(releaseText, manifestCommand) {
		t.Fatalf("release workflow does not generate basename checksums with %q", manifestCommand)
	}
	if strings.Contains(releaseText, "sha256sum dist/* > dist/checksums.txt") {
		t.Fatal("release workflow still writes dist-prefixed checksum paths")
	}
	const verifyCommand = `grep "  ${asset}$" checksums.txt | sha256sum -c -`
	if !strings.Contains(actionText, verifyCommand) {
		t.Fatal("action checksum verification no longer expects basename entries")
	}
	const smokeStepName = "        name: Verify release checksum manifest"
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
	if !strings.Contains(smokeRun, manifestCommand) {
		t.Fatal("CI smoke test does not generate the release manifest command")
	}
	if !strings.Contains(smokeRun, verifyCommand) {
		t.Fatal("CI smoke test does not use the action checksum verification command")
	}
	for _, asset := range []string{
		"devparity-linux-amd64",
		"devparity-linux-arm64",
		"devparity-darwin-amd64",
		"devparity-darwin-arm64",
		"devparity-windows-amd64.exe",
	} {
		if !strings.Contains(smokeRun, asset) {
			t.Fatalf("CI smoke test does not cover asset %q", asset)
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

	if !strings.Contains(verifyText, "workflow_call:") {
		t.Fatal("verify workflow is not reusable")
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
		"Verify release checksum manifest",
		"DEVPARITY_CONTAINER_TEST: \"1\"",
	} {
		if !strings.Contains(verifyText, gate) {
			t.Fatalf("verification workflow missing %q", gate)
		}
	}
	if !strings.Contains(ciText, "uses: ./.github/workflows/verify.yml") {
		t.Fatal("CI does not call reusable verification workflow")
	}
	if !strings.Contains(releaseText, "verify:\n    uses: ./.github/workflows/verify.yml") {
		t.Fatal("release does not call reusable verification workflow")
	}
	if !strings.Contains(releaseText, "package:\n    needs: verify") {
		t.Fatal("package job does not wait for verification")
	}
	if !strings.Contains(releaseText, "ref: ${{ github.sha }}") {
		t.Fatal("release package job does not check out the caller commit")
	}
	if !strings.Contains(releaseText, "publish:\n    needs: [verify, package]") {
		t.Fatal("publish does not wait for verification and packaging")
	}
	if !strings.Contains(releaseText, "permissions:\n      contents: write\n    steps:") {
		t.Fatal("publish job lacks isolated write permission")
	}
	if !strings.Contains(releaseText, "name: release-assets") || !strings.Contains(releaseText, "name: release-assets\n          path: dist") {
		t.Fatal("release assets are not passed between package and publish")
	}
	if !strings.Contains(releaseText, "actions/download-artifact@v7") {
		t.Fatal("publish job does not download release assets")
	}
	if !strings.Contains(releaseText, "gh release create") {
		t.Fatal("publish job does not create the release")
	}
	if strings.Contains(verifyText, "contents: write") {
		t.Fatal("verification workflow has write permission")
	}
}
