package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	gv := crt.GroupVersion.String()

	workspaces := crt.WorkspaceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "WorkspaceList",
			APIVersion: gv,
		},
		Items: []crt.Workspace{
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workspace",
					APIVersion: gv,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "user-ns1",
				},
				Status: crt.WorkspaceStatus{
					Namespaces: []crt.SpaceNamespace{
						{
							Name: "user-ns1",
							Type: "default",
						},
					},
					Owner: "user1",
					Role:  "admin",
				},
			},
			{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workspace",
					APIVersion: gv,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "user-ns2",
				},
				Status: crt.WorkspaceStatus{
					Namespaces: []crt.SpaceNamespace{
						{
							Name: "user-ns2",
							Type: "default",
						},
					},
					Owner: "user1",
					Role:  "admin",
				},
			},
		},
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
