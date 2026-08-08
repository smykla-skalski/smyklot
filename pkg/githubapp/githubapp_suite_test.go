package githubapp_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGitHubApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GitHubApp Suite")
}
