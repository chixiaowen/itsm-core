package logger

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestNew_Variants(t *testing.T) {
	cases := []struct {
		level, format, output string
		wantErr               bool
	}{
		{"info", "console", "stdout", false},
		{"debug", "json", "stdout", false},
		{"warn", "console", "stderr", false},
		{"error", "json", "stderr", false},
		{"", "", "", false},
		{"warning", "console", "stdout", false},
		{"bogus", "console", "stdout", true},
	}
	for _, c := range cases {
		z, err := New(c.level, c.format, c.output)
		if c.wantErr {
			if err == nil {
				t.Fatalf("level=%q 应报错", c.level)
			}
			continue
		}
		if err != nil {
			t.Fatalf("level=%q err=%v", c.level, err)
		}
		z.Info("test", zap.String("k", "v"))
	}
}

func TestNew_FileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	z, err := New("info", "json", path)
	if err != nil {
		t.Fatalf("文件输出构造失败: %v", err)
	}
	z.Info("hello-file")
	_ = z.Sync()
}

func TestNew_InvalidFilePath(t *testing.T) {
	// 指向一个不存在的目录
	if _, err := New("info", "console", "/nonexistent-dir-xyz/app.log"); err == nil {
		t.Fatalf("非法文件路径应报错")
	}
}
