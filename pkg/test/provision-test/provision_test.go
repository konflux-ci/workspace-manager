package provision_test

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/workspace-manager/pkg/test/utils"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestProvision(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Provision namespace Suite")
}

var k8sClient client.Client
var testEnv *envtest.Environment
var serverProcess *exec.Cmd
var serverCancelFunc context.CancelFunc

var _ = BeforeSuite(func() {
	schema := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(schema))
	testEnv = &envtest.Environment{BinaryAssetsDirectory: "../../../bin/k8s/1.29.0-linux-amd64/"}
	k8sClient = utils.StartTestEnv(schema, testEnv)
	serverProcess, serverCancelFunc = utils.CreateWorkspaceManagerServer("../../../cmd/main.go", nil, "")
	utils.WaitForWorkspaceManagerServerToServe()
})

var _ = AfterSuite(func() {
	utils.StopWorkspaceManagerServer(serverProcess, serverCancelFunc)
	utils.StopEnvTest(testEnv)
})

var _ = Describe("simple test", func() {
	endpoint := "http://localhost:5000/api/v1/signup"
	httpClient := &http.Client{}

	Context("simple test context", func() {
		It("simple spec", func() {
			request, err := http.NewRequest("GET", endpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			Eventually(
				func() (int, error) {
					response, err := httpClient.Do(request)
					if err != nil {
						fmt.Println(err.Error())
						return 0, err
					}
					return response.StatusCode, nil

				},
				10,
				1,
			).Should(Equal(http.StatusOK))
		})
	})
})
