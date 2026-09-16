package asset

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

func TestHandler_ErrorPaths(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.CmdbManager)
	_ = e.seedAsset(t, "AST-0001", "笔记本")

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodPut, "/api/v1/assets/abc", http.StatusBadRequest},
		{http.MethodPut, "/api/v1/assets/999", http.StatusNotFound},
		{http.MethodGet, "/api/v1/assets/999", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/assets/abc", http.StatusBadRequest},
		{http.MethodDelete, "/api/v1/assets/999", http.StatusNotFound},
		{http.MethodGet, "/api/v1/assets/abc/history", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/assets/999/history", http.StatusNotFound},
		{http.MethodPost, "/api/v1/assets/abc/transition", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/assets/999/bind-ci", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/assets/999/bind-ci", http.StatusNotFound},
	}
	for _, tc := range cases {
		var body any
		switch {
		case tc.method == http.MethodPut:
			body = AssetRequest{AssetNo: "x", Name: "x", Category: "laptop"}
		case tc.path == "/api/v1/assets/abc/transition":
			body = TransitionRequest{Action: ActionStockIn}
		case tc.path == "/api/v1/assets/999/bind-ci":
			body = BindCIRequest{CIID: 100}
		}
		if w := doJSON(t, r, tc.method, tc.path, body); w.Code != tc.want {
			t.Fatalf("%s %s 期望 %d，实际 %d (%s)", tc.method, tc.path, tc.want, w.Code, w.Body.String())
		}
	}

	// 未绑定 CI 时解绑 → 422
	if w := doJSON(t, r, http.MethodDelete, "/api/v1/assets/1/bind-ci", nil); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("未绑定解绑应 422，实际 %d", w.Code)
	}
}

func TestGetAssetDetail_WithCI(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a := e.seedAsset(t, "AST-0001", "笔记本")
	if _, err := e.svc.BindCI(ctx, op(), a.ID, 100); err != nil {
		t.Fatalf("绑定失败: %v", err)
	}
	d, err := e.svc.GetAssetDetail(ctx, a.ID)
	if err != nil {
		t.Fatalf("详情失败: %v", err)
	}
	if d.CI == nil || d.CI.ID != 100 {
		t.Fatalf("详情应含绑定 CI: %+v", d.CI)
	}
}

func TestListHistory_NotFound(t *testing.T) {
	e := newTestEnv(t)
	if _, err := e.svc.ListHistory(context.Background(), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestModel_TableNamesAndServiceActions(t *testing.T) {
	if (Asset{}).TableName() != "assets" {
		t.Fatalf("Asset 表名错误")
	}
	if (AssetHistory{}).TableName() != "asset_histories" {
		t.Fatalf("AssetHistory 表名错误")
	}
	e := newTestEnv(t)
	if acts := e.svc.AssetActions(StatusPlanned); len(acts) != 1 {
		t.Fatalf("planned 应有 1 个动作，实际 %v", acts)
	}
}

func TestHandler_BadJSONBodies(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.Admin)
	for _, path := range []string{"/api/v1/assets/1/transition", "/api/v1/assets/1/bind-ci"} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString("{bad"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s 非法 JSON 应 400，实际 %d", path, w.Code)
		}
	}
}
