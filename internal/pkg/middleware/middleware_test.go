package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

func newEngine(mws ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mws...)
	return r
}

func serve(r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestRequestID_GenerateAndPassthrough(t *testing.T) {
	r := newEngine(RequestID())
	r.GET("/x", func(c *gin.Context) {
		if RequestIDOf(c) == "" {
			c.String(http.StatusInternalServerError, "no rid")
			return
		}
		c.String(http.StatusOK, "ok")
	})

	w := serve(r, http.MethodGet, "/x", "")
	if w.Code != http.StatusOK {
		t.Fatalf("应自动生成 request id，实际 %d", w.Code)
	}
	if w.Header().Get(RequestIDHeader) == "" {
		t.Fatalf("响应头应回填 X-Request-ID")
	}

	// 透传
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "trace-123")
	r.ServeHTTP(w2, req)
	if w2.Header().Get(RequestIDHeader) != "trace-123" {
		t.Fatalf("应透传 X-Request-ID，实际 %s", w2.Header().Get(RequestIDHeader))
	}
}

func TestRequestIDOf_Absent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if RequestIDOf(c) != "" {
		t.Fatalf("未设置时应返回空串")
	}
}

func TestLogger_Levels(t *testing.T) {
	z := zap.NewNop()
	r := newEngine(Logger(z))
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/bad", func(c *gin.Context) { c.String(http.StatusBadRequest, "bad") })
	r.GET("/err", func(c *gin.Context) { c.String(http.StatusInternalServerError, "err") })

	for _, p := range []string{"/ok", "/bad", "/err"} {
		if w := serve(r, http.MethodGet, p, ""); w.Code == 0 {
			t.Fatalf("%s 无响应", p)
		}
	}
	// nil logger 分支
	r2 := newEngine(Logger(nil))
	r2.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	serve(r2, http.MethodGet, "/ok", "")
}

func TestRecovery_Panic(t *testing.T) {
	r := newEngine(Recovery(nil))
	r.GET("/boom", func(_ *gin.Context) { panic("kaboom") })
	w := serve(r, http.MethodGet, "/boom", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panic 应 500，实际 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "50000") {
		t.Fatalf("响应体应含错误码 50000: %s", w.Body.String())
	}
}

func TestRecovery_PanicAfterWrite(t *testing.T) {
	r := newEngine(Recovery(zap.NewNop()))
	r.GET("/partial", func(c *gin.Context) {
		c.String(http.StatusOK, "partial")
		panic("late panic")
	})
	w := serve(r, http.MethodGet, "/partial", "")
	if w.Code != http.StatusOK {
		t.Fatalf("已写入响应后 panic 应保持原状态，实际 %d", w.Code)
	}
}

func TestCORS(t *testing.T) {
	r := newEngine(CORS([]string{"http://localhost:5173"}))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 允许来源
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("允许来源应回显 ACAO")
	}

	// 未授权来源不回显
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.Header.Set("Origin", "http://evil.example")
	r.ServeHTTP(w2, req2)
	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("未授权来源不应回显 ACAO")
	}

	// 预检
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req3.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNoContent {
		t.Fatalf("预检应 204，实际 %d", w3.Code)
	}

	// 允许所有（空白名单）
	rAll := newEngine(CORS(nil))
	rAll.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req4.Header.Set("Origin", "http://any.example")
	rAll.ServeHTTP(w4, req4)
	if w4.Header().Get("Access-Control-Allow-Origin") != "http://any.example" {
		t.Fatalf("空白名单应放行任意来源")
	}
}

func TestExtractBearer(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"BEARER abc":  "abc",
		"Bearer  abc": "abc",
		"abc":         "",
		"":            "",
		"Bearer":      "",
	}
	for in, want := range cases {
		if got := extractBearer(in); got != want {
			t.Fatalf("extractBearer(%q)=%q，期望 %q", in, got, want)
		}
	}
}

func TestAuth_Flow(t *testing.T) {
	mgr := security.NewJWTManager("secret", time.Hour)
	token, _, _ := mgr.Issue(42, "alice", role.Agent)

	r := newEngine(Auth(mgr, "/skip", "/healthz"))
	r.GET("/skip", func(c *gin.Context) { c.String(http.StatusOK, "skip") })
	r.GET("/me", func(c *gin.Context) {
		a, ok := GetActor(c)
		if !ok {
			c.String(http.StatusInternalServerError, "no actor")
			return
		}
		c.String(http.StatusOK, a.Username)
	})

	// 白名单免鉴权
	if w := serve(r, http.MethodGet, "/skip", ""); w.Code != http.StatusOK {
		t.Fatalf("白名单应放行，实际 %d", w.Code)
	}
	// 无 token -> 401
	if w := serve(r, http.MethodGet, "/me", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// 非法 token -> 401
	if w := serve(r, http.MethodGet, "/me", "bad.token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("非法 token 应 401，实际 %d", w.Code)
	}
	// 合法 token -> 200 + actor
	if w := serve(r, http.MethodGet, "/me", token); w.Code != http.StatusOK || w.Body.String() != "alice" {
		t.Fatalf("合法 token 应 200/alice，实际 %d/%s", w.Code, w.Body.String())
	}

	// 过期 token -> 401
	expiredMgr := security.NewJWTManager("secret", -time.Minute)
	expTok, _, _ := expiredMgr.Issue(1, "bob", role.Agent)
	if w := serve(r, http.MethodGet, "/me", expTok); w.Code != http.StatusUnauthorized {
		t.Fatalf("过期 token 应 401，实际 %d", w.Code)
	}

	// nil 管理器 + 有 token -> 500
	rNil := newEngine(Auth(nil))
	rNil.GET("/me", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	if w := serve(rNil, http.MethodGet, "/me", token); w.Code != http.StatusInternalServerError {
		t.Fatalf("nil 管理器应 500，实际 %d", w.Code)
	}
}

func TestRequirePermAndRole(t *testing.T) {
	mgr := security.NewJWTManager("secret", time.Hour)
	admin := tokenForRole(t, mgr, 1, role.Admin)
	requestor := tokenForRole(t, mgr, 2, role.Requestor)

	r := newEngine(Auth(mgr))
	r.GET("/manage", RequirePerm(role.PermUserManage), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/only-admin", RequireRole(role.Admin), func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	if w := serve(r, http.MethodGet, "/manage", admin); w.Code != http.StatusOK {
		t.Fatalf("admin 应 200，实际 %d", w.Code)
	}
	if w := serve(r, http.MethodGet, "/manage", requestor); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 应 403，实际 %d", w.Code)
	}
	if w := serve(r, http.MethodGet, "/only-admin", admin); w.Code != http.StatusOK {
		t.Fatalf("admin 应 200，实际 %d", w.Code)
	}
	if w := serve(r, http.MethodGet, "/only-admin", requestor); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 应 403，实际 %d", w.Code)
	}

	// 无 actor（未挂 Auth）-> 401
	rNoAuth := newEngine()
	rNoAuth.GET("/manage", RequirePerm(role.PermUserManage), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	if w := serve(rNoAuth, http.MethodGet, "/manage", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 actor 应 401，实际 %d", w.Code)
	}
	rNoAuth2 := newEngine()
	rNoAuth2.GET("/only-admin", RequireRole(role.Admin), func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	if w := serve(rNoAuth2, http.MethodGet, "/only-admin", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 actor 应 401，实际 %d", w.Code)
	}
}

func TestActorContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if _, ok := GetActor(c); ok {
		t.Fatalf("未设置时不应存在 actor")
	}
	SetActor(c, Actor{UserID: 7, Username: "u", Role: role.Agent})
	a, ok := GetActor(c)
	if !ok || a.UserID != 7 || a.Role != role.Agent {
		t.Fatalf("actor 读写不符: %+v ok=%v", a, ok)
	}
}

func tokenForRole(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return tok
}
