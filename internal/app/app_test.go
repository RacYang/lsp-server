// 应用装配测试：创建 App 并快速退出。
package app_test

import (
	"context"
	"testing"
	"time"

	"racoo.cn/lsp/internal/app"
	"racoo.cn/lsp/internal/config"
)

func TestNewAndShutdown(t *testing.T) {
	cfg := config.Config{ServerAddr: "127.0.0.1:0", RuleID: "sichuan_xzdd"}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = a.Run(ctx)
}
