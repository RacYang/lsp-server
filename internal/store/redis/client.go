package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client 封装 go-redis 客户端，集中承载会话、幂等键与路由缓存访问。
type Client struct {
	kv goredis.UniversalClient
}

// NewClient 通过 Redis 地址创建数据面访问器；addr 为空视为配置错误。
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("empty redis addr")
	}
	cli := goredis.NewClient(&goredis.Options{Addr: addr})
	return NewClientFromUniversal(cli), nil
}

// NewClientFromUniversal 允许测试注入自定义客户端（如 miniredis 对应的普通 client）。
func NewClientFromUniversal(cli goredis.UniversalClient) *Client {
	if cli == nil {
		return nil
	}
	return &Client{kv: cli}
}

// Ping 用于启动期探活与集成测试连通性确认。
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("nil redis client")
	}
	return c.kv.Ping(ctx).Err()
}

// Close 关闭底层连接池；重复调用时按 go-redis 语义返回 nil。
func (c *Client) Close() error {
	if c == nil || c.kv == nil {
		return nil
	}
	return c.kv.Close()
}

// ttlOrDefault 将无效 TTL 归一为较短默认值，避免把临时状态写成永久键。
func ttlOrDefault(ttl time.Duration, fallback time.Duration) time.Duration {
	if ttl <= 0 {
		return fallback
	}
	return ttl
}
