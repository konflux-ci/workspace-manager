package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/labstack/echo/v4"
	k8sapi "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	authorizationv1Client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"context"
	"net/http/httptest"
	"testing"

	crt "github.com/codeready-toolchain/api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/konflux-ci/workspace-manager/pkg/test/utils"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

type HTTPResponse struct {
	Body       string
	StatusCode int
}

type HTTPheader struct {
	name  string
	value string
}

type NamespaceRoleBinding struct {
	Namespace   string
	Role        string
	RoleBinding string
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
		"Calling the workspace endpoint for funcuser1 responds only with the 'func-test-tenant' workspace info",
		HTTPheader{"X-Email", "funcuser1@konflux.dev"},
		http.StatusOK,
		`{"kind":"WorkspaceList","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":{},`+
			`"items":[{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1",`+
			`"metadata":{"name":"func-test-tenant","creationTimestamp":null},"status":`+
			`{"namespaces":[{"name":"func-test-tenant","type":"default"}]}}]}`),
	Entry(
		"Workspace endpoint for funcuser2 responds with 2 namespaces info",
		HTTPheader{"X-Email", "funcuser2@konflux.dev"},
		http.StatusOK,
		`{"kind":"WorkspaceList","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":{},`+
			`"items":[{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1",`+
			`"metadata":{"name":"func-test-tenant","creationTimestamp":null},"status":{"namespaces":`+
			`[{"name":"func-test-tenant","type":"default"}]}},{"kind":"Workspace","apiVersion":`+
			`"toolchain.dev.openshift.com/v1alpha1","metadata":{"name":"func-test-tenant-2",`+
			`"creationTimestamp":null},"status":{"namespaces":[{"name":"func-test-tenant-2",`+
			`"type":"default"}]}}]}`),
	Entry(
		"Workspace endpoint for funcuser3 responds with no namespaces",
		HTTPheader{"X-Email", "funcuser3@konflux.dev"},
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
		"Calling the workspace endpoint for the func-test-tenant workspace for funcuser2",
		"func-test-tenant",
		HTTPheader{"X-Email", "funcuser2@konflux.dev"},
		http.StatusOK,
		`{"kind":"Workspace","apiVersion":"toolchain.dev.openshift.com/v1alpha1","metadata":`+
			`{"name":"func-test-tenant","creationTimestamp":null},"status":{"namespaces":`+
			`[{"name":"func-test-tenant","type":"default"}]}}`),
	Entry(
		"Specific workspace endpoint for func-test-tenant-2 for funcuser1 only",
		"func-test-tenant-2",
		HTTPheader{"X-Email", "funcuser1@konflux.dev"},
		404,
		`{"message":"Not Found"}`),
)

var serverProcess *exec.Cmd
var serverCancelFunc context.CancelFunc

var _ = BeforeSuite(func() {
	schema := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(schema))
	testEnv = &envtest.Environment{BinaryAssetsDirectory: "../bin/k8s/1.29.0-linux-amd64/"}
	k8sClient = utils.StartTestEnv(schema, testEnv)

	serverProcess, serverCancelFunc = utils.CreateWorkspaceManagerServer("main.go", nil, "")
	utils.WaitForWorkspaceManagerServerToServe()

	user1 := "funcuser1@konflux.dev"
	user2 := "funcuser2@konflux.dev"
	namespaceNames := []string{"func-test-tenant", "func-test-tenant-2", "func-test-tenant-3"}
	for _, name := range namespaceNames {
		_, err := createNamespace(k8sClient, name)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
	}
	createRole(k8sClient, "func-test-tenant", "func-namespace-access", []string{"create", "list", "watch", "delete"})
	createRole(k8sClient, "func-test-tenant-2", "func-namespace-access-2", []string{"create", "list", "watch", "delete"})
	createRoleBinding(k8sClient, "func-namespace-access-user-binding", "func-test-tenant", user1, "func-namespace-access")
	createRoleBinding(k8sClient, "func-namespace-access-user-binding-2", "func-test-tenant", user2, "func-namespace-access")
	createRoleBinding(k8sClient, "func-namespace-access-user-binding-3", "func-test-tenant-2", user2, "func-namespace-access-2")
})

var _ = AfterSuite(func() {
	utils.StopWorkspaceManagerServer(serverProcess, serverCancelFunc)
	utils.StopEnvTest(testEnv)
})

var _ = Describe("TestRunAccessCheck", func() {
	var authCl authorizationv1Client.AuthorizationV1Interface

	BeforeEach(func() {
		// Set up Kubernetes client
		cfg, err := config.GetConfig()
		Expect(err).NotTo(HaveOccurred(), "Unexpected error getting Kubernetes config")
		clientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred(), "Unexpected error creating Kubernetes clientset")
		authCl = clientset.AuthorizationV1()
	})

	Context("When a user has access to the resource", func() {
		It("should return true for a user with 'create' permission on test-tenant", func() {
			user := "user3@konflux.dev"
			namespace := "test-tenant"
			resource := "applications"
			verb := "create"
			expectedResult := true
			_, err := createNamespace(k8sClient, namespace)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", namespace, err))
			createRole(k8sClient, "test-tenant", "namespace-access", []string{"create", "list", "watch", "delete"})
			createRoleBinding(k8sClient, "namespace-access-user-binding", "test-tenant", user, "namespace-access")
			boolresult, err := runAccessCheck(authCl, user, namespace, "appstudio.redhat.com", resource, verb)
			Expect(boolresult).To(Equal(expectedResult))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing RunAccessCheck")
		})
	})

	Context("When a user does not have any permissions on the namespace", func() {
		It("should return false for a user without access to test-tenant-2", func() {
			user := "user4@konflux.dev"
			namespace := "test-tenant-2"
			resource := "applications"
			verb := "create"
			expectedResult := false
			_, err := createNamespace(k8sClient, namespace)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", namespace, err))

			createRole(k8sClient, "test-tenant-2", "namespace-access-2", []string{"create", "list", "watch", "delete"})
			createRoleBinding(k8sClient, "namespace-access-user-binding-3", "test-tenant-2", user, "namespace-access-2")
			boolresult, err := runAccessCheck(authCl, "user3@konflux.dev", namespace, "appstudio.redhat.com", resource, verb)
			Expect(boolresult).To(Equal(expectedResult))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing RunAccessCheck")
		})
	})

	Context("When a user lacks the specific action permission on the namespace", func() {
		It("should return false for a user without 'patch' permission on test-tenant-1", func() {
			user := "user5@konflux.dev"
			namespace := "test-tenant-1"
			resource := "applications"
			verb := "patch"
			expectedResult := false
			_, err := createNamespace(k8sClient, namespace)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", namespace, err))
			createRole(k8sClient, "test-tenant-1", "namespace-access", []string{"create", "list", "watch", "delete"})
			createRoleBinding(k8sClient, "namespace-access-user-binding", "test-tenant-1", user, "namespace-access")
			boolresult, err := runAccessCheck(authCl, user, namespace, "appstudio.redhat.com", resource, verb)
			Expect(boolresult).To(Equal(expectedResult))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing RunAccessCheck")
		})
	})
})

var _ = Describe("GetWorkspacesWithAccess querying for workspaces with access", func() {
	var (
		allNamespaces      []k8sapi.Namespace
		expectedWorkspaces []crt.Workspace
		e                  *echo.Echo
		c                  echo.Context
		gv                 string
	)

	BeforeEach(func() {
		e = echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c = e.NewContext(req, rec)
		c.Request().Header.Set("X-Email", "user@konflux.dev")
	})

	Context("When workspace test-tenant's namespaces has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-1", "ws-test-tenant-2"}
		BeforeEach(func() {
			gv = crt.GroupVersion.String()
			for _, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
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
			mockNamespaceWithAccess := func(e *echo.Echo, c echo.Context, authCl authorizationv1Client.AuthorizationV1Interface, allNamespaces []k8sapi.Namespace) ([]k8sapi.Namespace, error) {
				return []k8sapi.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ws-test-tenant-1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ws-test-tenant-2",
						},
					},
				}, nil
			}
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces, mockNamespaceWithAccess)
			Expect(actualWorkspaces.Items).To(Equal(expectedWorkspaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})
	})

	Context("When workspace with only ws-test-tenant-3 namespace has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-3", "ws-test-tenant-4"}
		BeforeEach(func() {
			gv = crt.GroupVersion.String()
			for _, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
			}
			expectedWorkspaces = []crt.Workspace{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Workspace",
						APIVersion: gv,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ws-test-tenant-3",
					},
					Status: crt.WorkspaceStatus{
						Namespaces: []crt.SpaceNamespace{
							{
								Name: "ws-test-tenant-3",
								Type: "default",
							},
						},
					},
				},
			}
		})
		It("Should return a WorkspaceList with test-tenant workspace and only test-tenant namespace in it", func() {
			mockNamespaceWithAccess := func(e *echo.Echo, c echo.Context, authCl authorizationv1Client.AuthorizationV1Interface, allNamespaces []k8sapi.Namespace) ([]k8sapi.Namespace, error) {
				return []k8sapi.Namespace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ws-test-tenant-3",
						},
					},
				}, nil
			}
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces, mockNamespaceWithAccess)
			Expect(actualWorkspaces.Items).To(Equal(expectedWorkspaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})
	})

	Context("When no workspaces has all the necessary permissions", func() {
		namespaceNames := []string{"ws-test-tenant-5", "ws-test-tenant-6"}
		BeforeEach(func() {
			for _, name := range namespaceNames {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s: %v", name, err))
				allNamespaces = append(allNamespaces, ns)
			}
		})
		It("Should return a empty WorkspaceList", func() {
			mockNamespaceWithAccess := func(e *echo.Echo, c echo.Context, authCl authorizationv1Client.AuthorizationV1Interface, allNamespaces []k8sapi.Namespace) ([]k8sapi.Namespace, error) {
				return []k8sapi.Namespace{}, nil
			}
			actualWorkspaces, err := getWorkspacesWithAccess(e, c, allNamespaces, mockNamespaceWithAccess)
			Expect(actualWorkspaces.Items).To(BeEmpty())
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetWorkspacesWithAccess")
		})
	})
})

var _ = Describe("TestGetNamespacesWithAccess", func() {
	var (
		allNamespaces, actualNs, expectedNs []k8sapi.Namespace
		err                                 error
		mappings                            []NamespaceRoleBinding
		e                                   *echo.Echo
		authCl                              authorizationv1Client.AuthorizationV1Interface
		c                                   echo.Context
	)

	BeforeEach(func() {
		e = echo.New()
		cfg, err := config.GetConfig()
		Expect(err).NotTo(HaveOccurred(), "Error getting Kubernetes config")

		clientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred(), "Error creating Kubernetes client")

		authCl = clientset.AuthorizationV1()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c = e.NewContext(req, rec)
		c.Request().Header.Set("X-Email", "user@konflux.dev")
	})

	Context("When all the namespaces have the necessary permissions", func() {
		mappings = []NamespaceRoleBinding{
			{
				Namespace:   "ns-test-tenant-1",
				Role:        "ns-namespace-access-1",
				RoleBinding: "ns-namespace-access-user-binding-1",
			},
			{
				Namespace:   "ns-test-tenant-2",
				Role:        "ns-namespace-access-2",
				RoleBinding: "ns-namespace-access-user-binding-2",
			},
		}
		BeforeEach(func() {
			for _, name := range mappings {
				ns, err := createNamespace(k8sClient, name.Namespace)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s", name.Namespace))
				allNamespaces = append(allNamespaces, ns)
				createRole(k8sClient, name.Namespace, name.Role, []string{"create", "list", "watch", "delete"})
				createRoleBinding(k8sClient, name.RoleBinding, name.Namespace, "user@konflux.dev", name.Role)
			}
		})
		It("returns all namespaces in the list", func() {
			actualNs, err = getNamespacesWithAccess(e, c, authCl, allNamespaces)
			Expect(actualNs).To(Equal(allNamespaces))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetNamespacesWithAccess")
		})
		AfterEach(func() {
			allNamespaces = nil
		})
	})

	Context("When none of the namspaces have necessary permissions", func() {
		var name = "ns-test-tenant-3"
		BeforeEach(func() {
			ns3, err := createNamespace(k8sClient, name)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s", name))
			allNamespaces = []k8sapi.Namespace{ns3}
		})
		It("doesn't return any namespace", func() {
			actualNs, err = getNamespacesWithAccess(e, c, authCl, allNamespaces)
			Expect(actualNs).To(BeEmpty())
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetNamespacesWithAccess")
		})
		AfterEach(func() {
			allNamespaces = nil
		})
	})

	Context("When namspace ns-test-tenant-5 doesn't have necessary permissions", func() {
		BeforeEach(func() {
			var names = []string{"ns-test-tenant-5", "ns-test-tenant-6"}
			for _, name := range names {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error while creating the namespace %s", name))
				allNamespaces = append(allNamespaces, ns)
				expectedNs = []k8sapi.Namespace{ns}
			}
			createRole(k8sClient, "ns-test-tenant-6", "ns-namespace-access-6", []string{"create", "list", "watch", "delete"})
			createRoleBinding(k8sClient, "ns-namespace-access-user-binding-6", "ns-test-tenant-6", "user@konflux.dev", "ns-namespace-access-6")
		})
		It("only returns ns-test-tenant-6 namespace", func() {
			actualNs, err = getNamespacesWithAccess(e, c, authCl, allNamespaces)
			Expect(actualNs).To(Equal(expectedNs))
			Expect(err).NotTo(HaveOccurred(), "Unexpected error testing GetNamespacesWithAccess")
		})
		AfterEach(func() {
			allNamespaces = nil
		})
	})
})

var _ = Describe("GetUserNamespaces", func() {
	var e *echo.Echo
	var createdNamespaces []string

	// checks if all created namespaces are in the returned list
	Context("When querying for all user namespaces using Exists", func() {
		It("Should return all created namespaces", func() {
			namesToCreate := []string{"test-ns-1", "test-ns-2", "test-ns-3"}
			for _, name := range namesToCreate {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating the namespace %s", name))
				createdNamespaces = append(createdNamespaces, ns.Name)
			}

			req, err := labels.NewRequirement("kubernetes.io/metadata.name", selection.Exists, []string{})
			Expect(err).NotTo(HaveOccurred(), "Error creating label requirement")

			namespaces, err := getUserNamespaces(e, *req)
			Expect(err).NotTo(HaveOccurred(), "Error getting user namespaces")

			var actualNamespaces []string
			for _, ns := range namespaces {
				actualNamespaces = append(actualNamespaces, ns.Name)
			}

			for _, createdNs := range createdNamespaces {
				Expect(actualNamespaces).To(ContainElement(createdNs))
			}
		})

		AfterEach(func() {
			createdNamespaces = nil
		})
	})
	// checks if specific namespaces are in the returned list
	Context("When querying for specific namespaces using In", func() {
		It("Should return only the specified namespaces", func() {
			for _, name := range []string{"in-test-1", "in-test-2", "not-in-test"} {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating the namespace %s", name))
				createdNamespaces = append(createdNamespaces, ns.Name)
			}

			req, err := labels.NewRequirement("kubernetes.io/metadata.name", selection.In, []string{"in-test-1", "in-test-2"})
			Expect(err).NotTo(HaveOccurred(), "Error creating label requirement")

			namespaces, err := getUserNamespaces(e, *req)
			Expect(err).NotTo(HaveOccurred(), "Error getting user namespaces")

			var actualNamespaces []string
			for _, ns := range namespaces {
				actualNamespaces = append(actualNamespaces, ns.Name)
			}

			Expect(actualNamespaces).To(ConsistOf("in-test-1", "in-test-2"))
			Expect(actualNamespaces).NotTo(ContainElement("not-in-test"))
		})
	})

	Context("When querying for namespaces using NotIn", func() {
		It("Should return namespaces not in the specified list", func() {
			for _, name := range []string{"ts-keep-1", "ts-keep-2", "ts-exclude-1", "ts-exclude-2"} {
				ns, err := createNamespace(k8sClient, name)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating the namespace %s", name))
				createdNamespaces = append(createdNamespaces, ns.Name)
			}

			req, err := labels.NewRequirement("kubernetes.io/metadata.name", selection.NotIn, []string{"ts-exclude-1", "ts-exclude-2"})
			Expect(err).NotTo(HaveOccurred(), "Error creating label requirement")

			namespaces, err := getUserNamespaces(e, *req)
			Expect(err).NotTo(HaveOccurred(), "Error getting user namespaces")

			var actualNamespaces []string
			for _, ns := range namespaces {
				actualNamespaces = append(actualNamespaces, ns.Name)
			}

			Expect(actualNamespaces).To(ContainElements("ts-keep-1", "ts-keep-2"))
			Expect(actualNamespaces).NotTo(ContainElements("ts-exclude-1", "ts-exclude-2"))
		})
	})
})
