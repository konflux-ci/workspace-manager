package provisioner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	rbacv1 "k8s.io/api/rbac/v1"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handler Unit Tests")
}

var _ = DescribeTable(
	"TestNormalizeEmail",
	func(input string, expected string) {
		result := normalizeEmail(input)
		Expect(result).To(Equal(expected))
	},
	Entry(
		"simple-email",
		"user@konflux.dev",
		"user-konflux-dev-tenant",
	),
	Entry(
		"email with a .",
		"user.name@konflux.dev",
		"user-name-konflux-dev-tenant",
	),
	Entry(
		"email with a + sign",
		"user+konflux@konflux.dev",
		"user-konflux-konflux-dev-tenant",
	),
	Entry(
		"result is longer than 63 and should be trimmed",
		"user+aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@konflux.dev",
		"user-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-tenant",
	),
)

var _ = DescribeTable(
	"Test CheckNSExistHandler",
	func(k8sClient client.Client, expectedStatus int) {
		nsp := NewNSProvisioner(k8sClient)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Email", "user@konflux.dev")
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)
		err := nsp.CheckNSExistHandler(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(rec.Result().StatusCode).Error().To(Equal(expectedStatus))
	},
	Entry(
		"Error when getting a namespace",
		fakeclient.
			NewClientBuilder().
			WithInterceptorFuncs(
				interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return errors.New("fake client error")
					},
				},
			).Build(),
		http.StatusInternalServerError,
	),
	Entry(
		"Namespace was not found",
		fakeclient.
			NewClientBuilder().
			WithInterceptorFuncs(
				interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return kubeerr.NewNotFound(
							schema.GroupResource{
								Group:    "",
								Resource: "Namespace",
							},
							"ns1",
						)
					},
				},
			).Build(),
		http.StatusNotFound,
	),
)

var _ = DescribeTable(
	"Test CheckNSExistHandler",
	func(k8sClient client.Client, expectedStatus int) {
		nsp := NewNSProvisioner(k8sClient)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Email", "user@konflux.dev")
		req.Header.Set("X-User", "user1")
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)
		err := nsp.CreateNSHandler(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(rec.Result().StatusCode).Error().To(Equal(expectedStatus))
	},
	Entry(
		"Error when creating a namespace",
		fakeclient.
			NewClientBuilder().
			WithInterceptorFuncs(
				interceptor.Funcs{
					Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.New("Failed to create namespace")
					},
				},
			).Build(),
		http.StatusInternalServerError,
	),
	Entry(
		"Error when creating rolebinding",
		fakeclient.
			NewClientBuilder().
			WithInterceptorFuncs(
				interceptor.Funcs{
					Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*rbacv1.RoleBinding); ok {
							return errors.New("Failed to create rolebinding")
						}

						return nil
					},
				},
			).Build(),
		http.StatusInternalServerError,
	),
)
