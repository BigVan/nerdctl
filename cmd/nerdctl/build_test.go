/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestBuild(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()
	ignoredImageNamed := imageName + "-" + "ignored"
	outputOpt := fmt.Sprintf("--output=type=docker,name=%s", ignoredImageNamed)
	base.Cmd("build", buildCtx, "-t", imageName, outputOpt).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutExactly("nerdctl-build-test-string\n")
	base.Cmd("run", "--rm", ignoredImageNamed).AssertFail()
}

// TestBuildBaseImage tests if an image can be built on the previously built image.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildBaseImage(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	imageName2 := imageName + "-2"
	defer base.Cmd("rmi", imageName2).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN echo hello > /hello
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName).AssertOK()

	dockerfile2 := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]
	`, imageName)

	buildCtx2, err := createBuildContext(dockerfile2)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx2)

	base.Cmd("build", "-t", imageName2, buildCtx2).AssertOK()
	base.Cmd("build", buildCtx2, "-t", imageName2).AssertOK()

	base.Cmd("run", "--rm", imageName2).AssertOutExactly("hello2\n")
}

// TestBuildFromContainerd tests if an image can be built on an image pulled by nerdctl.
// This isn't currently supported by nerdctl with BuildKit OCI worker.
func TestBuildFromContainerd(t *testing.T) {
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()
	imageName2 := imageName + "-2"
	defer base.Cmd("rmi", imageName2).Run()

	// FIXME: BuildKit sometimes tries to use base image manifests of platforms that hasn't been
	//        pulled by `nerdctl pull`. This leads to "not found" error for the base image.
	//        To avoid this issue, images shared to BuildKit should always be pulled by manifest
	//        digest or `--all-platforms` needs to be added.
	base.Cmd("pull", "--all-platforms", testutil.CommonImage).AssertOK()
	base.Cmd("tag", testutil.CommonImage, imageName).AssertOK()
	base.Cmd("rmi", testutil.CommonImage).AssertOK()

	dockerfile2 := fmt.Sprintf(`FROM %s
RUN echo hello2 > /hello2
CMD ["cat", "/hello2"]
	`, imageName)

	buildCtx2, err := createBuildContext(dockerfile2)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx2)

	base.Cmd("build", "-t", imageName2, buildCtx2).AssertOK()
	base.Cmd("build", buildCtx2, "-t", imageName2).AssertOK()

	base.Cmd("run", "--rm", imageName2).AssertOutExactly("hello2\n")
}

func TestBuildFromStdin(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-stdin"]
	`, testutil.CommonImage)

	base.Cmd("build", "-t", imageName, "-f", "-", ".").CmdOption(testutil.WithStdin(strings.NewReader(dockerfile))).AssertCombinedOutContains(imageName)
}

func TestBuildWithDockerfile(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-dockerfile"]
	`, testutil.CommonImage)

	buildCtx := filepath.Join(t.TempDir(), "test")
	err := os.MkdirAll(buildCtx, 0755)
	assert.NilError(t, err)
	err = os.WriteFile(filepath.Join(buildCtx, "Dockerfile"), []byte(dockerfile), 0644)
	assert.NilError(t, err)

	pwd, err := os.Getwd()
	assert.NilError(t, err)
	err = os.Chdir(buildCtx)
	assert.NilError(t, err)
	defer os.Chdir(pwd)

	// hack os.Getwd return "(unreachable)" on rootless
	t.Setenv("PWD", buildCtx)

	base.Cmd("build", "-t", imageName, "-f", "Dockerfile", "..").AssertOK()
	base.Cmd("build", "-t", imageName, "-f", "Dockerfile", ".").AssertOK()
	// fail err: no such file or directory
	base.Cmd("build", "-t", imageName, "-f", "../Dockerfile", ".").AssertFail()
}

func TestBuildLocal(t *testing.T) {
	t.Parallel()
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	const testFileName = "nerdctl-build-test"
	const testContent = "nerdctl"
	outputDir := t.TempDir()

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /`,
		testFileName)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	if err := os.WriteFile(filepath.Join(buildCtx, testFileName), []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	testFilePath := filepath.Join(outputDir, testFileName)
	base.Cmd("build", "-o", fmt.Sprintf("type=local,dest=%s", outputDir), buildCtx).AssertOK()
	if _, err := os.Stat(testFilePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(testFilePath)
	assert.NilError(t, err)
	assert.Equal(t, string(data), testContent)
}

func createBuildContext(dockerfile string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "nerdctl-build-test")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return "", err
	}
	return tmpDir, nil
}

func TestBuildWithIIDFile(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)
	fileName := filepath.Join(t.TempDir(), "id.txt")

	base.Cmd("build", "-t", imageName, buildCtx, "--iidfile", fileName).AssertOK()
	base.Cmd("build", buildCtx, "-t", imageName, "--iidfile", fileName).AssertOK()
	defer os.Remove(fileName)

	imageID, err := os.ReadFile(fileName)
	assert.NilError(t, err)

	base.Cmd("run", "--rm", string(imageID)).AssertOutExactly("nerdctl-build-test-string\n")
}

func TestBuildWithLabels(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	imageName := testutil.Identifier(t)

	dockerfile := fmt.Sprintf(`FROM %s
LABEL name=nerdctl-build-test-label
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx, "--label", "label=test").AssertOK()
	defer base.Cmd("rmi", imageName).Run()

	base.Cmd("inspect", imageName, "--format", "{{json .Config.Labels }}").AssertOutExactly("{\"label\":\"test\",\"name\":\"nerdctl-build-test-label\"}\n")
}

func TestBuildMultipleTags(t *testing.T) {
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	defer base.Cmd("builder", "prune").Run()
	img := testutil.Identifier(t)
	imgWithNoTag, imgWithCustomTag := fmt.Sprintf("%s%d", img, 2), fmt.Sprintf("%s%d:hello", img, 3)
	defer base.Cmd("rmi", img).Run()
	defer base.Cmd("rmi", imgWithNoTag).Run()
	defer base.Cmd("rmi", imgWithCustomTag).Run()

	dockerfile := fmt.Sprintf(`FROM %s
CMD ["echo", "nerdctl-build-test-string"]
	`, testutil.CommonImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", img, buildCtx).AssertOK()
	base.Cmd("build", buildCtx, "-t", img, "-t", imgWithNoTag, "-t", imgWithCustomTag).AssertOK()
	base.Cmd("run", "--rm", img).AssertOutExactly("nerdctl-build-test-string\n")
	base.Cmd("run", "--rm", imgWithNoTag).AssertOutExactly("nerdctl-build-test-string\n")
	base.Cmd("run", "--rm", imgWithCustomTag).AssertOutExactly("nerdctl-build-test-string\n")
}
