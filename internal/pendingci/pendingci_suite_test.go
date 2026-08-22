package pendingci_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPendingCI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pending CI Suite")
}
