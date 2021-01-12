// +build tools

package tools

import (
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "golang.org/x/tools/cmd/goimports"
	_ "mvdan.cc/sh/v3/cmd/shfmt"
)

// This file imports packages that are used when running go generate, or used
// during the development process but not otherwise depended on by built code.
