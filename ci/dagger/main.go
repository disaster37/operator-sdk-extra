// A generated module for OperatorSdkExtra functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"context"
	"regexp"

	"dagger/operator-sdk-extra/internal/dagger"

	"emperror.dev/errors"
)

const (
	kubeVersion          = "1.36.0"
	sdkVersion           = "v1.37.0"
	controllerGenVersion = "v0.16.1"
	kustomizeVersion     = "v5.4.3"
	cleanCrdVersion      = "v0.1.9"
	opmVersion           = "v1.48.0"
	gitUsername          = "github"
	gitEmail             = "github@localhost"
	defaultBranch        = "main"
)

// branchNameRegex guards the git ref name used for the push-back (defense in
// depth; the workflow validates the same pattern before invoking the module).
// It rejects empty names, leading dashes (git argument injection), whitespace,
// control characters and shell metacharacters.
var branchNameRegex = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

type OperatorSdkExtra struct {
	// +private
	Src *dagger.Directory

	// +private
	OperatorSDK *dagger.OperatorSDK

	// +private
	Golang *dagger.Golang
}

func New(
	// The source directory
	// +required
	src *dagger.Directory,
) *OperatorSdkExtra {
	cleanDir := src.WithoutDirectory("ci").WithoutDirectory("samples")
	return &OperatorSdkExtra{
		Src:         src,
		OperatorSDK: dag.OperatorSDK(cleanDir, "operator-sdk-extra"),
		Golang:      dag.Golang(cleanDir),
	}
}

func (h *OperatorSdkExtra) Test(
	ctx context.Context,
	// if only short running tests should be executed
	// +optional
	short bool,
	// if the tests should be executed out of order
	// +optional
	shuffle bool,
	// run select tests only, defined using a regex
	// +optional
	run string,
	// skip select tests, defined using a regex
	// +optional
	skip string,
	// Run test with gotestsum
	// +optional
	withGotestsum bool,
	// Path to test
	// +optional
	path string,
) *dagger.File {
	return h.OperatorSDK.Golang().Test(dagger.OperatorSDKGolangTestOpts{
		Short:           short,
		Shuffle:         shuffle,
		Run:             run,
		Skip:            skip,
		WithGotestsum:   withGotestsum,
		Path:            path,
		WithKubeversion: kubeVersion,
	})
}

// Release permit to release to operator version
func (h *OperatorSdkExtra) CI(
	ctx context.Context,

	// Set true to run tests
	// +optional
	ci bool,

	// Set true to skip test
	// +optional
	skipTest bool,

	// The git token
	// +optional
	gitToken *dagger.Secret,

	// The codecov token
	// +optional
	codecovToken *dagger.Secret,

	// The git branch to push generated code back to.
	// Pass it with `--git-branch env:CI_GIT_BRANCH`: the Dagger CLI resolves the
	// environment variable on the host, so the ref name is never interpolated
	// into argv. Falls back to the default branch when omitted.
	// +optional
	gitBranch *dagger.Secret,

	// The Git repository URL to push generated code to.
	// +optional
	// +default="https://github.com/disaster37/operator-sdk-extra.git"
	gitRepoURL string,
) (*dagger.Directory, error) {
	var dir *dagger.Directory
	var err error

	// Generate manifests
	dir = h.OperatorSDK.SDK().GenerateManifests()
	h.OperatorSDK = h.OperatorSDK.WithSource(dir)
	h.Golang = h.Golang.WithSource(dir)

	// Format code
	dir = h.Golang.Format()
	h.OperatorSDK = h.OperatorSDK.WithSource(dir)
	h.Golang = h.Golang.WithSource(dir)

	// Lint code
	if _, err = h.Golang.Lint(ctx); err != nil {
		return nil, errors.Wrap(err, "Error when lint code")
	}

	// Vuln check
	if _, err = h.Golang.Vulncheck(ctx); err != nil {
		return nil, errors.Wrap(err, "Error when check vulnerability")
	}

	// Test code
	if !skipTest {
		coverageFile := h.Test(
			ctx,
			false,
			false,
			"",
			"",
			true,
			"",
		)
		dir = dir.WithFile("coverage.out", coverageFile)
	}

	if ci {

		if gitToken == nil {
			return nil, errors.New("You must provide '--git-token'")
		}

		if codecovToken == nil {
			return nil, errors.New("You must provide '--codecov-token'")
		}

		// Codecov
		if _, err = dag.Codecov().Upload(
			ctx,
			dir,
			codecovToken,
			dagger.CodecovUploadOpts{
				Name:  "disaster37/operator-sdk-extra",
				Files: []string{"coverage.out"},
			},
		); err != nil {
			return nil, errors.Wrap(err, "Error when run Codecov upload")
		}

		// Add all folder remove previously
		dir = dir.
			WithDirectory("ci", h.Src.Directory("ci")).
			WithDirectory("samples", h.Src.Directory("samples"))

		// Commit / push
		git := dag.GitModule(dir, dagger.GitModuleOpts{Ci: "github"}).
			SetConfig(dagger.GitModuleSetConfigOpts{
				Username: gitUsername,
				Email:    gitEmail,
			})

		// The branch to push back to is passed as a secret (resolved from the
		// CI_GIT_BRANCH environment variable by the Dagger CLI on the host), so
		// the ref name is never interpolated into argv. Dagger does not forward
		// host environment variables into module functions, hence the secret
		// indirection. See .github/workflows/ci.yaml.
		branchName := defaultBranch
		if gitBranch != nil {
			branch, err := gitBranch.Plaintext(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "Error when read git branch")
			}
			if branch != "" {
				branchName = branch
			}
		}
		if !branchNameRegex.MatchString(branchName) {
			return nil, errors.Errorf("Unsafe git branch name: %q", branchName)
		}

		if _, err = git.CommitAndPush(
			ctx,
			gitToken,
			dagger.GitModuleCommitAndPushOpts{
				BranchName: branchName,
				GitRepoURL: gitRepoURL,
				Message:    "Commit from CI",
			},
		); err != nil {
			return nil, errors.Wrap(err, "Error when commit and push files change")
		}
	}

	return dir, nil
}
