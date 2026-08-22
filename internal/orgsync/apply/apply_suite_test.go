package apply

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApply(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Org Sync Apply Suite")
}
