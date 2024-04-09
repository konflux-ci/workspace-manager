package main

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	crt "github.com/codeready-toolchain/api/api/v1alpha1"
)

type SignupStatusReason = string

var SignedUp SignupStatusReason = "SignedUp"

type SignupStatus struct {
	Ready  bool               `json:"ready"`
	Reason SignupStatusReason `json:"reason"`
}

type Signup struct {
	SignupStatus SignupStatus `json:"status"`
}

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	e := echo.New()

	e.Pre(middleware.RemoveTrailingSlash())

	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.POST("/api/v1/signup", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/api/v1/signup", func(c echo.Context) error {
		resp := &Signup{
			SignupStatus: SignupStatus{
				Ready:  true,
				Reason: SignedUp,
			},
		}
		return c.JSON(http.StatusOK, resp)
	})

	cfg, err := config.GetConfig()
	if err != nil {
		e.Logger.Fatal(err)
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		e.Logger.Fatal(err)
	}
	namespaceList := &core.NamespaceList{}
	err = cl.List(
		context.Background(),
		namespaceList,
	)
	if err != nil {
		e.Logger.Fatal(err)
	}

	gv := crt.GroupVersion.String()

	var wss []crt.Workspace

	for _, ns := range namespaceList.Items {
		ws := crt.Workspace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Workspace",
				APIVersion: gv,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: ns.Name,
			},
			Status: crt.WorkspaceStatus{
				Namespaces: []crt.SpaceNamespace{
					{
						Name: ns.Name,
						Type: "default",
					},
				},
				// Owner: "user1",
				// Role:  "admin",
			},
		}
		wss = append(wss, ws)
	}

	workspaces := crt.WorkspaceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "WorkspaceList",
			APIVersion: gv,
		},
		Items: wss,
	}

	e.GET("/workspaces", func(c echo.Context) error {
		return c.JSON(http.StatusOK, &workspaces)
	})

	e.GET("/workspaces/:ws", func(c echo.Context) error {
		wsParam := c.Param("ws")
		for _, ws := range workspaces.Items {
			if ws.Name == wsParam {
				return c.JSON(http.StatusOK, &ws)
			}
		}

		return echo.NewHTTPError(http.StatusNotFound)
	})

	e.Logger.Fatal(e.Start(":5000"))
}
