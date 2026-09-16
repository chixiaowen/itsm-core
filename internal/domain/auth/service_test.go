package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

// fakeUserRepo 是 platform.UserRepository 的内存 fake。
type fakeUserRepo struct {
	byID   map[uint64]*platform.User
	byName map[string]*platform.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uint64]*platform.User{}, byName: map[string]*platform.User{}}
}

func (r *fakeUserRepo) add(u *platform.User) {
	r.byID[u.ID] = u
	r.byName[u.Username] = u
}

func (r *fakeUserRepo) Create(_ context.Context, u *platform.User) error {
	r.add(u)
	return nil
}

func (r *fakeUserRepo) Update(_ context.Context, u *platform.User) error {
	r.add(u)
	return nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uint64) (*platform.User, error) {
	if u, ok := r.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, platform.ErrNotFound
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*platform.User, error) {
	if u, ok := r.byName[username]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, platform.ErrNotFound
}

func (r *fakeUserRepo) Delete(_ context.Context, id uint64) error {
	u, ok := r.byID[id]
	if !ok {
		return platform.ErrNotFound
	}
	delete(r.byID, id)
	delete(r.byName, u.Username)
	return nil
}

func (r *fakeUserRepo) List(_ context.Context, _ platform.UserListQuery) ([]platform.User, int64, error) {
	out := make([]platform.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, *u)
	}
	return out, int64(len(out)), nil
}

// ListOptions 满足扩展后的 platform.UserRepository 契约（auth 域未用到）。
func (r *fakeUserRepo) ListOptions(_ context.Context, roleName string) ([]platform.UserOption, error) {
	out := make([]platform.UserOption, 0, len(r.byID))
	for _, u := range r.byID {
		if u.Status != platform.UserStatusActive {
			continue
		}
		if roleName != "" && u.Role != roleName {
			continue
		}
		out = append(out, platform.UserOption{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role})
	}
	return out, nil
}

func seededRepo(t *testing.T) *fakeUserRepo {
	t.Helper()
	hash, err := security.HashPassword("secret1")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	repo := newFakeUserRepo()
	repo.add(&platform.User{ID: 1, Username: "alice", Role: role.Agent, PasswordHash: hash, Status: platform.UserStatusActive})
	repo.add(&platform.User{ID: 2, Username: "bob", Role: role.Agent, PasswordHash: hash, Status: platform.UserStatusDisabled})
	return repo
}

func appErrCode(t *testing.T, err error) int {
	t.Helper()
	var ae *httpx.AppError
	if err == nil {
		t.Fatalf("期望错误，实际 nil")
	}
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *httpx.AppError，实际 %T", err)
	}
	return ae.Code
}

// ---------------- service 层 ----------------

func TestService_Login(t *testing.T) {
	repo := seededRepo(t)
	svc := NewService(repo, security.NewJWTManager("secret", time.Hour))
	ctx := context.Background()

	// 成功
	res, err := svc.Login(ctx, LoginRequest{Username: "alice", Password: "secret1"})
	if err != nil {
		t.Fatalf("登录应成功: %v", err)
	}
	if res.Token == "" || res.User == nil || res.User.ID != 1 {
		t.Fatalf("登录结果不符: %+v", res)
	}

	// 错误密码
	if _, err := svc.Login(ctx, LoginRequest{Username: "alice", Password: "wrong"}); appErrCode(t, err) != httpx.CodeUnauthorized {
		t.Fatalf("错误密码应 401，实际 %v", err)
	}
	// 未知用户
	if _, err := svc.Login(ctx, LoginRequest{Username: "ghost", Password: "x"}); appErrCode(t, err) != httpx.CodeUnauthorized {
		t.Fatalf("未知用户应 401")
	}
	// 禁用账号
	if _, err := svc.Login(ctx, LoginRequest{Username: "bob", Password: "secret1"}); appErrCode(t, err) != httpx.CodeUnauthorized {
		t.Fatalf("禁用账号应 401")
	}
}

func TestService_Me(t *testing.T) {
	repo := seededRepo(t)
	svc := NewService(repo, security.NewJWTManager("secret", time.Hour))
	ctx := context.Background()

	if u, err := svc.Me(ctx, 1); err != nil || u.ID != 1 {
		t.Fatalf("Me 应成功: %v", err)
	}
	if _, err := svc.Me(ctx, 2); appErrCode(t, err) != httpx.CodeUnauthorized {
		t.Fatalf("禁用账号 Me 应 401")
	}
	if _, err := svc.Me(ctx, 999); appErrCode(t, err) != httpx.CodeUnauthorized {
		t.Fatalf("不存在用户 Me 应 401")
	}
}

func TestService_NotInitialized(t *testing.T) {
	svc := NewService(nil, nil)
	if _, err := svc.Login(context.Background(), LoginRequest{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("未初始化应 500")
	}
	if _, err := svc.Me(context.Background(), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("未初始化应 500")
	}
}

// ---------------- HTTP 层（httptest） ----------------

func newAuthEngine(t *testing.T) (*gin.Engine, *security.JWTManager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := seededRepo(t)
	jwtMgr := security.NewJWTManager("secret", time.Hour)
	h := NewHandler(NewService(repo, jwtMgr))

	r := gin.New()
	r.Use(middleware.RequestID())
	public := r.Group("/api/v1")
	Register(public, h)
	protected := r.Group("/api/v1", middleware.Auth(jwtMgr, "/api/v1/auth/login", "/healthz"))
	RegisterProtected(protected, h)
	return r, jwtMgr
}

func doJSON(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAuthHTTP_LoginMeLogout(t *testing.T) {
	r, jwtMgr := newAuthEngine(t)

	// 登录成功
	w := doJSON(r, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"secret1"}`, "")
	if w.Code != http.StatusOK {
		t.Fatalf("登录应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	if resp.Code != 0 || resp.Data.Token == "" {
		t.Fatalf("登录响应不符: %s", w.Body.String())
	}

	// 错误密码 401
	if w := doJSON(r, http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"bad"}`, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401，实际 %d", w.Code)
	}
	// 参数缺失 400
	if w := doJSON(r, http.MethodPost, "/api/v1/auth/login", `{}`, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("缺参应 400，实际 %d", w.Code)
	}

	// 无 token 访问 /me -> 401
	if w := doJSON(r, http.MethodGet, "/api/v1/auth/me", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// 带 token 访问 /me -> 200
	if w := doJSON(r, http.MethodGet, "/api/v1/auth/me", "", resp.Data.Token); w.Code != http.StatusOK {
		t.Fatalf("带 token 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 失效 token -> 401
	if w := doJSON(r, http.MethodGet, "/api/v1/auth/me", "", "invalid.token.here"); w.Code != http.StatusUnauthorized {
		t.Fatalf("非法 token 应 401，实际 %d", w.Code)
	}

	// 登出（携带有效 token）-> 200
	token, _, _ := jwtMgr.Issue(1, "alice", role.Agent)
	if w := doJSON(r, http.MethodPost, "/api/v1/auth/logout", "", token); w.Code != http.StatusOK {
		t.Fatalf("登出应 200，实际 %d", w.Code)
	}
}

func TestAuthHTTP_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := seededRepo(t)
	expiredMgr := security.NewJWTManager("secret", -time.Minute)
	h := NewHandler(NewService(repo, security.NewJWTManager("secret", time.Hour)))

	r := gin.New()
	protected := r.Group("/api/v1", middleware.Auth(expiredMgr))
	RegisterProtected(protected, h)

	token, _, _ := expiredMgr.Issue(1, "alice", role.Agent)
	if w := doJSON(r, http.MethodGet, "/api/v1/auth/me", "", token); w.Code != http.StatusUnauthorized {
		t.Fatalf("过期 token 应 401，实际 %d", w.Code)
	}
}
