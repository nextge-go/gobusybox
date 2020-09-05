// Copyright 2015-2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package golang is an API to the Go compiler.
package golang

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Environ struct {
	build.Context

	GO111MODULE string
}

// Default is the default build environment comprised of the default GOPATH,
// GOROOT, GOOS, GOARCH, and CGO_ENABLED values.
func Default() Environ {
	return Environ{
		Context:     build.Default,
		GO111MODULE: os.Getenv("GO111MODULE"),
	}
}

// GoCmd runs a go command in the environment.
func (c Environ) GoCmd(args ...string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(c.GOROOT, "bin", "go"), args...)
	cmd.Env = append(os.Environ(), c.Env()...)
	return cmd
}

// Version returns the Go version string that runtime.Version would return for
// the Go compiler in this environ.
func (c Environ) Version() (string, error) {
	cmd := c.GoCmd("version")
	v, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	s := strings.Fields(string(v))
	if len(s) < 3 {
		return "", fmt.Errorf("unknown go version, tool returned weird output for 'go version': %v", string(v))
	}
	return s[2], nil
}

// Env returns all environment variables for invoking a Go command.
func (c Environ) Env() []string {
	var env []string
	if c.GOARCH != "" {
		env = append(env, fmt.Sprintf("GOARCH=%s", c.GOARCH))
	}
	if c.GOOS != "" {
		env = append(env, fmt.Sprintf("GOOS=%s", c.GOOS))
	}
	if c.GOPATH != "" {
		env = append(env, fmt.Sprintf("GOPATH=%s", c.GOPATH))
	}
	var cgo int8
	if c.CgoEnabled {
		cgo = 1
	}
	env = append(env, fmt.Sprintf("CGO_ENABLED=%d", cgo))
	env = append(env, fmt.Sprintf("GO111MODULE=%s", c.GO111MODULE))

	if c.GOROOT != "" {
		env = append(env, fmt.Sprintf("GOROOT=%s", c.GOROOT))

		// If GOROOT is set to a different version of Go, we must
		// ensure that $GOROOT/bin is also in path to make the "go"
		// binary available to golang.org/x/tools/packages.
		env = append(env, fmt.Sprintf("PATH=%s:%s", filepath.Join(c.GOROOT, "bin"), os.Getenv("PATH")))
	}
	return env
}

// String returns all environment variables for Go invocations.
func (c Environ) String() string {
	return strings.Join(c.Env(), " ")
}

// Optional arguments to Environ.Build.
type BuildOpts struct {
	// NoStrip builds an unstripped binary.
	NoStrip bool
	// ExtraArgs to `go build`.
	ExtraArgs []string
}

// BuildDir compiles the package in the directory `dirPath`, writing the build
// object to `binaryPath`.
func (c Environ) BuildDir(dirPath string, binaryPath string, opts BuildOpts) error {
	args := []string{
		"build",

		// Force rebuilding of packages.
		"-a",

		// Strip all symbols, and don't embed a Go build ID to be reproducible.
		"-ldflags", "-s -w -buildid=",

		"-o", binaryPath,
		"-installsuffix", "uroot",

		"-gcflags=all=-l", // Disable "function inlining" to get a smaller binary
	}
	if !opts.NoStrip {
		args = append(args, `-ldflags=-s -w`) // Strip all symbols.
	}

	v, err := c.Version()
	if err != nil {
		return err
	}

	// Reproducible builds: Trim any GOPATHs out of the executable's
	// debugging information.
	//
	// E.g. Trim /tmp/bb-*/ from /tmp/bb-12345567/src/github.com/...
	if strings.Contains(v, "go1.13") || strings.Contains(v, "go1.14") || strings.Contains(v, "gotip") {
		args = append(args, "-trimpath")
	} else {
		args = append(args, "-gcflags", fmt.Sprintf("-trimpath=%s", c.GOPATH))
		args = append(args, "-asmflags", fmt.Sprintf("-trimpath=%s", c.GOPATH))
	}

	if len(c.BuildTags) > 0 {
		args = append(args, []string{"-tags", strings.Join(c.BuildTags, " ")}...)
	}
	// We always set the working directory, so this is always '.'.
	args = append(args, ".")

	cmd := c.GoCmd(args...)
	cmd.Dir = dirPath

	if o, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error building go package in %q: %v, %v", dirPath, string(o), err)
	}
	return nil
}