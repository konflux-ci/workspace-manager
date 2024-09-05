package dummy

import (
	"net/http"

	"github.com/konflux-ci/workspace-manager/pkg/api/v1alpha1"
	"github.com/labstack/echo/v4"
)

func DummySignupPostHandler(c echo.Context) error {
	return c.String(http.StatusOK, "ok")
}

func DummySignupGetHandler(c echo.Context) error {
	resp := &v1alpha1.Signup{
		SignupStatus: v1alpha1.SignupStatus{
			Ready:  true,
			Reason: v1alpha1.SignedUp,
		},
	}
	return c.JSON(http.StatusOK, resp)
}
