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

	"dagger/operator-sdk-extra/internal/dagger"

	"emperror.dev/errors"
	"github.com/disaster37/dagger-library-go/lib/helper"
)

const (
	kubeVersion          = "1.31.0"
	sdkVersion           = "v1.37.0"
	controllerGenVersion = "v0.16.1"
	kustomizeVersion     = "v5.4.3"
	cleanCrdVersion      = "v0.1.9"
	opmVersion           = "v1.48.0"
	gitUsername          = "github"
	gitEmail             = "github@localhost"
	defaultBranch        = "main"
)

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
		OperatorSDK: dag.OperatorSDK(cleanDir),
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

	// Set true if current build is a tag
	// It will use the stable and alpha channel
	// alpha channel only instead
	// +optional
	isTag bool,

	// Set true if current build is a Pull request
	// +optional
	isPullRequest bool,

	// Set the current branch name. It's needed because of CI overwrite the branch name by PR
	// +optional
	branchName string,

	// Set true to skip test
	// +optional
	skipTest bool,

	// The git token
	// +optional
	gitToken *dagger.Secret,

	// The codeCov token
	// +optional
	codeCoveToken *dagger.Secret,
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

		// Put ci folder to not lost it

		// codecov
		if _, err := dag.Codecov().Upload(
			ctx,
			dir,
			codeCoveToken,
			dagger.CodecovUploadOpts{
				Files:               []string{"coverage.out"},
				Verbose:             true,
				InjectCiEnvironment: true,
			},
		); err != nil {
			return nil, errors.Wrap(err, "Error when upload report on CodeCov")
		}

		// Commit / push
		var branch string
		git := dag.Git().
			SetConfig(gitUsername, gitEmail, dagger.GitSetConfigOpts{BaseRepoURL: "github.com", Token: gitToken})

		if !isTag {
			if branchName == "" {
				return nil, errors.New("You need to provide the branch name")
			}
			branch = branchName
		} else {
			branch = defaultBranch
		}

		if isPullRequest {
			git = git.With(func(r *dagger.Git) *dagger.Git {
				ctr := r.BaseContainer().
					WithDirectory("/project", dir).
					WithWorkdir("/project").
					WithExec(helper.ForgeCommand("git remote -v")).
					WithExec(helper.ForgeCommandf("git fetch origin %s:%s", branch, branch)).
					WithExec(helper.ForgeCommandf("git checkout %s", branch))

				return r.WithCustomContainer(ctr)
			})
		} else {
			git = git.SetRepo(h.Src.WithDirectory(".", dir), dagger.GitSetRepoOpts{Branch: branch})
		}
		if _, err = git.CommitAndPush(ctx, "Commit from CI pipeline"); err != nil {
			return nil, errors.Wrap(err, "Error when commit and push files change")
		}
	}

	return dir, nil
}
