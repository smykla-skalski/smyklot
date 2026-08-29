package filerender_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileRender(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File Render Suite")
}
