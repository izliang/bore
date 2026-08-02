package cyberhuma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/e"
	"github.com/izliang/bore/web/router"
)

type testGetInput struct{}

type testGetOutput struct {
	Name string `json:"name"`
}

func TestWithTagsAddsOpenAPITags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := New(engine, WithHumaDocs(router.DocConfig{Exposure: router.DocAlways, UI: router.DocUINone}))

	GET[testGetInput, testGetOutput](api, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "test"}, nil
	}, WithTags("用户管理"))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]struct {
			Tags []string `json:"tags"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	operation := spec.Paths["/users"]["get"]
	if len(operation.Tags) != 1 || operation.Tags[0] != "用户管理" {
		t.Fatalf("expected tags [用户管理], got %#v", operation.Tags)
	}
}

func TestNewAcceptsGinRouterGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := New(engine.Group("/api/v1"), WithHumaDocs(router.DocConfig{Exposure: router.DocAlways, UI: router.DocUINone}))

	GET[testGetInput, testGetOutput](api, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "test"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected grouped route status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected root route status 404, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected grouped openapi status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	if _, ok := spec.Paths["/users"]; !ok {
		t.Fatalf("expected openapi path /users, got %#v", spec.Paths)
	}
}

func TestNewRouterGroupAppliesGinMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	apiGroup := engine.Group("/api/v1", func(c *gin.Context) {
		c.Header("X-Group", "api")
	})
	api := New(apiGroup)

	GET[testGetInput, testGetOutput](api, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "test"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Group") != "api" {
		t.Fatalf("expected group middleware header, got %q", recorder.Header().Get("X-Group"))
	}
}

func TestCyberhumaGroupRegistersRouteAndOpenAPIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := New(engine, WithHumaDocs(router.DocConfig{Exposure: router.DocAlways, UI: router.DocUINone}))
	admin := api.Group("/admin")

	GET[testGetInput, testGetOutput](admin, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "admin"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected grouped route status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected ungrouped route status 404, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected openapi status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	if _, ok := spec.Paths["/admin/users"]; !ok {
		t.Fatalf("expected openapi path /admin/users, got %#v", spec.Paths)
	}
	if _, ok := spec.Paths["/users"]; ok {
		t.Fatalf("did not expect ungrouped openapi path /users")
	}
}

func TestCyberhumaGroupGinMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := New(engine)
	admin := api.Group("/admin", WithGroupGinMiddleware(func(c *gin.Context) {
		c.Header("X-Admin", "true")
	}))

	GET[testGetInput, testGetOutput](api, "/public", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "public"}, nil
	})
	GET[testGetInput, testGetOutput](admin, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "admin"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected grouped route status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Admin") != "true" {
		t.Fatalf("expected admin middleware header, got %q", recorder.Header().Get("X-Admin"))
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/public", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected public route status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Admin") != "" {
		t.Fatalf("did not expect admin middleware header on public route")
	}
}

func TestCyberhumaGroupOperationModifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := New(engine, WithHumaDocs(router.DocConfig{Exposure: router.DocAlways, UI: router.DocUINone}))
	admin := api.Group("/admin", WithGroupSimpleOperationModifier(func(o *huma.Operation) {
		o.Tags = append(o.Tags, "admin")
	}))

	GET[testGetInput, testGetOutput](admin, "/users", func(context.Context, *testGetInput) (*testGetOutput, e.ServiceError) {
		return &testGetOutput{Name: "admin"}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected openapi status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec struct {
		Paths map[string]map[string]struct {
			Tags []string `json:"tags"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}

	operation := spec.Paths["/admin/users"]["get"]
	if len(operation.Tags) != 1 || operation.Tags[0] != "admin" {
		t.Fatalf("expected tags [admin], got %#v", operation.Tags)
	}
}
