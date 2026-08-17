package filemerge_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileMerge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File merge")
}
