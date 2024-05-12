package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	k8sapi "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type HTTPResponse struct {
	Body       string
	StatusCode int
}

type HTTPheader struct {
	name  string
	value string
}

var k8sClient client.Client
var testEnv *envtest.Environment

func createRole(k8sClient client.Client, nsName string, roleName string) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: nsName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"appstudio.redhat.com"},
				Resources: []string{"applications", "components"},
				Verbs:     []string{"create", "list", "watch", "delete"},
			},
		},
	}
	err := k8sClient.Create(context.Background(), role)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating 'Role' resource: %v", err))
}

func createRoleBinding(k8sClient client.Client, bindingName string, nsName string, userName string, roleName string) {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: nsName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     userName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	err := k8sClient.Create(context.Background(), roleBinding)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating 'roleBinding' resource: %v", err))
}

func createNamespace(k8sClient client.Client, name string) {
	namespaced := &k8sapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"konflux.ci/type":             "user",
				"kubernetes.io/metadata.name": name,
			},
		},
	}
	err := k8sClient.Create(context.Background(), namespaced)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating 'Namespace' resource: %v", err))
}

func performHTTPGetCall(url string, header HTTPheader) (*HTTPResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating request: %s", err)
		return nil, err
	}
	if header.name != "" {
		req.Header.Add(header.name, header.value)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request: %s", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body:  %s", err)
		return nil, err
	}
	response := &HTTPResponse{
		Body:       string(body),
		StatusCode: resp.StatusCode,
	}
	return response, nil
}

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}

var _ = Describe("Signup endpoint", func() {
	Context("Calling the signup endpoint with GET", func() {
		It("responds with ready and signedup", func() {
			url := "http://localhost:5000/api/v1/signup"
			expectedCode := http.StatusOK
			expectedBody := `{"status":{"ready":true,"reason":"SignedUp"}}`

			resp, err := performHTTPGetCall(url, HTTPheader{})
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Unexpected error testing the \"%s\" endpoint: %v", url, err))
			Expect(resp.StatusCode).To(Equal(expectedCode))
			Expect(strings.TrimSpace(expectedBody)).To(Equal(strings.TrimSpace(resp.Body)))
		})
	})
})

var _ = DescribeTable("Workspace endpoint", func(header HTTPheader, expectedCode int, expectedBody string) {
	url := "http://localhost:5000/workspaces"
	resp, err := performHTTPGetCall(url, header)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Unexpected error testing the \"%s\" endpoint: %v", url, err))
	Expect(resp.StatusCode).To(Equal(expectedCode))
	Expect(strings.TrimSpace(expectedBody)).To(Equal(strings.TrimSpace(resp.Body)))
},
	Entry(
		"Calling the workspace endpoint for user1 responds only with the 'test-tenant' workspace info",
		HTTPheader{"X-Email", "user1@konflux.dev"},
		http.StatusOK,
		`{"kind":"WorkspaceList","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":{},`+
			`"items":[{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1",`+
			`"metadata":{"name":"test-tenant","creationTimestamp":null},"status":`+
			`{"namespaces":[{"name":"test-tenant","type":"default"}]}}]}`),
	Entry(
		"Workspace endpoint for user2 responds with 2 namespaces info",
		HTTPheader{"X-Email", "user2@konflux.dev"},
		http.StatusOK,
		`{"kind":"WorkspaceList","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":{},`+
			`"items":[{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1",`+
			`"metadata":{"name":"test-tenant","creationTimestamp":null},"status":{"namespaces":`+
			`[{"name":"test-tenant","type":"default"}]}},{"kind":"Workspace","apiVersion":`+
			`"toolchain.dev.openshift.com/v1alpha1","metadata":{"name":"test-tenant-2",`+
			`"creationTimestamp":null},"status":{"namespaces":[{"name":"test-tenant-2",`+
			`"type":"default"}]}}]}`),
	Entry(
		"Workspace endpoint for user3 responds with no namespaces",
		HTTPheader{"X-Email", "user3@konflux.dev"},
		http.StatusOK,
		`{"kind":"WorkspaceList","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":{},"items":null}`),
	Entry(
		"Workspace endpoint with no header",
		HTTPheader{},
		500,
		`{"message":"Internal Server Error"}`),
)

var _ = DescribeTable("Specific workspace endpoint", func(endpoint string, header HTTPheader, expectedCode int, expectedBody string) {
	url := "http://localhost:5000/workspaces/" + endpoint
	resp, err := performHTTPGetCall(url, header)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Unexpected error testing the \"%s\" endpoint: %v", url, err))
	Expect(resp.StatusCode).To(Equal(expectedCode))
	Expect(strings.TrimSpace(expectedBody)).To(Equal(strings.TrimSpace(resp.Body)))
},
	Entry(
		"Calling the workspace endpoint for the test-tenant workspace for user2",
		"test-tenant",
		HTTPheader{"X-Email", "user2@konflux.dev"},
		http.StatusOK,
		`{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":`+
			`{"name":"test-tenant","creationTimestamp":null},"status":{"namespaces":`+
			`[{"name":"test-tenant","type":"default"}]}}`),
	Entry(
		"Specific workspace endpoint for test-tenant-2 for user1 only",
		"test-tenant-2",
		HTTPheader{"X-Email", "user1@konflux.dev"},
		404,
		`{"message":"Not Found"}`),
)

func CreateKubeconfigFileForRestConfig(restConfig rest.Config) string {
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   restConfig.Host,
		CertificateAuthorityData: restConfig.CAData,
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  "default-cluster",
		AuthInfo: "default-user",
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos["default-user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restConfig.CertData,
		ClientKeyData:         restConfig.KeyData,
	}
	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	kubeConfigFile, _ := os.CreateTemp("", "kubeconfig")
	_ = clientcmd.WriteToFile(clientConfig, kubeConfigFile.Name())
	return kubeConfigFile.Name()
}

var serverProcess *exec.Cmd

var _ = BeforeSuite(func() {
	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating the envtest environment during test setup: %v", err))
	kubeconfigPath := CreateKubeconfigFileForRestConfig(*cfg)
	os.Setenv("KUBECONFIG", kubeconfigPath)
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating the client during test setup: %v", err))
	Expect(k8sClient).NotTo(BeNil())

	user1 := "user1@konflux.dev"
	user2 := "user2@konflux.dev"
	createNamespace(k8sClient, "test-tenant")
	createNamespace(k8sClient, "test-tenant-2")
	createNamespace(k8sClient, "test-tenant-3")
	createRole(k8sClient, "test-tenant", "namespace-access")
	createRole(k8sClient, "test-tenant-2", "namespace-access-2")
	createRoleBinding(k8sClient, "namespace-access-user-binding", "test-tenant", user1, "namespace-access")
	createRoleBinding(k8sClient, "namespace-access-user-binding-2", "test-tenant", user2, "namespace-access")
	createRoleBinding(k8sClient, "namespace-access-user-binding-3", "test-tenant-2", user2, "namespace-access-2")
	serverProcess = exec.Command("go", "run", "main.go")
	err = serverProcess.Start()
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error starting the server during test setup: %v", err))
	time.Sleep(5 * time.Second)
})

var _ = AfterSuite(func() {
	Expect(os.Unsetenv("KUBECONFIG")).To(Succeed())
	if serverProcess != nil && serverProcess.Process != nil {
		err := serverProcess.Process.Kill()
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error killing the server during test teardown: %v", err))
	}
})
