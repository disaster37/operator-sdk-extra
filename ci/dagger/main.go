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
)

const (
	kubeVersion                 = "1.31.0"
	sdkVersion                  = "v1.37.0"
	controllerGenVersion        = "v0.16.1"
	kustomizeVersion            = "v5.4.3"
	cleanCrdVersion             = "v0.1.9"
	opmVersion                  = "v1.48.0"
	gitUsername          string = "github"
	gitEmail             string = "github@localhost"
	defaultBranch               = "main"
)

type OperatorSdkExtra struct {
	// +private
	Src *dagger.Directory

	// +private
	*dagger.OperatorSDK
}

func New(
	// The source directory
	// +required
	src *dagger.Directory,
) *OperatorSdkExtra {
	return &OperatorSdkExtra{
		Src:         src,
		OperatorSDK: dag.OperatorSDK(src.WithoutDirectory("ci").WithoutDirectory("samples")),
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
	return h.Golang().Test(dagger.OperatorSDKGolangTestOpts{
		Short:           short,
		Shuffle:         shuffle,
		Run:             run,
		Skip:            skip,
		WithGotestsum:   withGotestsum,
		Path:            path,
		WithKubeversion: kubeVersion,
	})
}
