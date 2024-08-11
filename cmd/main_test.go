package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	k8sapi "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"context"
	"os"
	"testing"

	crt "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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

func createRole(k8sClient client.Client, nsName string, roleName string, verbs []string) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: nsName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"appstudio.redhat.com"},
				Resources: []string{"applications", "components"},
				Verbs:     verbs,
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

func createNamespace(k8sClient client.Client, name string) (k8sapi.Namespace, error) {
	namespaced := &k8sapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"konflux.ci/type":             "user",
				"kubernetes.io/metadata.name": name,
			},
		},
	}
	if err := k8sClient.Create(context.Background(), namespaced); err != nil {
		return k8sapi.Namespace{}, fmt.Errorf("Error creating 'Namespace' resource: %v", err)
	}
	return *namespaced, nil
}

func deleteRole(k8sClient client.Client, nsName string, roleName string) {
	role := &rbacv1.Role{}
	role.Name = roleName
	role.Namespace = nsName
	err := k8sClient.Delete(context.Background(), role)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error deleting the role %s in namespace %s: %v\n", roleName, nsName, err))
}

func deleteRoleBinding(k8sClient client.Client, nsName string, roleBindingName string) {
	roleBinding := &rbacv1.RoleBinding{}
	roleBinding.Name = roleBindingName
	roleBinding.Namespace = nsName
	err := k8sClient.Delete(context.Background(), roleBinding)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error deleting the role binding %s in namespace %s: %v\n", roleBindingName, nsName, err))
}

func deleteNamespace(k8sClient client.Client, nsName string) {
	ns := &k8sapi.Namespace{}
	ns.Name = nsName
	err := k8sClient.Delete(context.Background(), ns)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error deleting the namespace: %s: %v\n", nsName, err))
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
	namespaceNames := []string{"test-tenant", "test-tenant-2", "test-tenant-3"}
	for _, name := range namespaceNames {
		_, err := createNamespace(k8sClient, name)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
	}
	createRole(k8sClient, "test-tenant", "namespace-access", []string{"create", "list", "watch", "delete"})
	createRole(k8sClient, "test-tenant-2", "namespace-access-2", []string{"create", "list", "watch", "delete"})
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

var _ = DescribeTable("TestRunAccessCheck", func(user string, namespace string, resource string, verb string, expectedResult bool) {
	cfg, _ := config.GetConfig()
	clientset, _ := kubernetes.NewForConfig(cfg)
	authCl := clientset.AuthorizationV1()

	boolresult, err := runAccessCheck(authCl, user, namespace, "appstudio.redhat.com", resource, verb)
	Expect(boolresult).To(Equal(expectedResult))
	Expect(err).NotTo(HaveOccurred(), "Unexpected error testing RunAccessCheck")
},
	Entry(
		"A user that has access to the resource should return true (user2 has permission to 'create' on test-tenant-1)",
		"user2@konflux.dev",
		"test-tenant",
		"applications",
		"create",
		true),
	Entry(
		"A user that does not have any premissions on the namespace should return false (user1 doesn't have access to test-tenant-2)",
		"user1@konflux.dev",
		"test-tenant-2",
		"applications",
		"create",
		false),
	Entry(
		"A user that does not have the permissions to perform the specific action on the namespace should return false (user1 doesn't have permission to 'patch' on test-tenant-1)",
		"user1@konflux.dev",
		"test-tenant-1",
		"applications",
		"patch",
		false),
)

var _ = DescribeTable("GetWorkspacesWithAccess querying for workspaces with access", func(gv string, allNamespaces []k8sapi.Namespace, expectedWorkspaces []crt.Workspace) {
	e := echo.New()
	c := e.NewContext(nil, nil)
	Context("When workspace test-tenant's namespaces has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-1", "ws-test-tenant-2"}
		roleNames := []string{"ws-namespace-access-1", "ws-namespace-access-2"}
		roleBindings := []string{"ws-namespace-access-user-binding-1", "ws-namespace-access-user-binding-2"}
		BeforeEach(func() {
			gv = "v1alpha1"
			for i, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
				createRole(k8sClient, name, roleNames[i], []string{"create", "list", "watch", "delete"})
				createRoleBinding(k8sClient, roleBindings[i], name, "user1@konflux.dev", roleNames[i])
			}			
			expectedWorkspaces = []crt.Workspace{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Workspace",
						APIVersion: gv,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ws-test-tenant-1",
					},
					Status: crt.WorkspaceStatus{
						Namespaces: []crt.SpaceNamespace{
							{
								Name: "ws-test-tenant-1",
								Type: "default",
							},
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Workspace",
						APIVersion: gv,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ws-test-tenant-2",
					},
					Status: crt.WorkspaceStatus{
						Namespaces: []crt.SpaceNamespace{
							{
								Name: "ws-test-tenant-2",
								Type: "default",
							},
						},
					},
				},
			}
		})

		It("Should return a WorkspaceList with test-tenant workspace and both namespaces in it", func() {
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces)
			Expect(actualWorkspaces.Items).To(Equal(expectedWorkspaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})

		AfterEach(func() {
			for i, name := range namespaceNames {
				deleteRoleBinding(k8sClient, name, roleBindings[i])
				deleteRole(k8sClient, name, roleNames[i])
				deleteNamespace(k8sClient, name)
			}
		})
	})

	Context("When workspace with only test-tenant namespace has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-1", "ws-test-tenant-2"}
		BeforeEach(func() {
			gv = "v1alpha1"
			for _, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
			}			
			createRole(k8sClient, "ws-test-tenant-1", "ws-namespace-access-1", []string{"create", "list", "watch", "delete"})
			createRoleBinding(k8sClient, "ws-namespace-access-user-binding-1", "ws-test-tenant-1", "user1@konflux.dev", "ws-namespace-access-1")
			expectedWorkspaces = []crt.Workspace{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Workspace",
						APIVersion: gv,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ws-test-tenant-1",
					},
					Status: crt.WorkspaceStatus{
						Namespaces: []crt.SpaceNamespace{
							{
								Name: "ws-test-tenant-1",
								Type: "default",
							},
						},
					},
				},
			}
		})

		It("Should return a WorkspaceList with test-tenant workspace and only test-tenant namespace in it", func() {
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces)
			Expect(actualWorkspaces.Items).To(Equal(expectedWorkspaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})

		AfterEach(func() {
			deleteRoleBinding(k8sClient, "ws-test-tenant-1", "ws-namespace-access-user-binding-1")
			deleteRole(k8sClient, "ws-test-tenant-1", "ws-namespace-access-1")
			for _, name := range namespaceNames {
				deleteNamespace(k8sClient, name)
			}
		})
	})	

	Context("When no workspaces has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-1", "ws-test-tenant-2"}
		BeforeEach(func() {
			gv = "v1alpha1"
			for _, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
			}
			createRole(k8sClient, "ws-test-tenant-1", "ws-namespace-access-1", []string{"create", "list"})
			createRoleBinding(k8sClient, "ws-namespace-access-user-binding-1", "ws-test-tenant-1", "user1@konflux.dev", "ws-namespace-access-1")
			expectedWorkspaces = []crt.Workspace{}
		})

		It("Should return a empty WorkspaceList", func() {
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces)
			Expect(actualWorkspaces.Items).To(Equal(expectedWorkspaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})

		AfterEach(func() {
			deleteRoleBinding(k8sClient, "ws-test-tenant-1", "ws-namespace-access-user-binding-1")
			deleteRole(k8sClient, "ws-test-tenant-1", "ws-namespace-access-1")
			for _, name := range namespaceNames {
				deleteNamespace(k8sClient, name)
			}
		})
	})
})
