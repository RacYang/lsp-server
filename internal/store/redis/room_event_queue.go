package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// 房间事件队列 TTL：防止宕机房间的积压事件无限占用内存。
const roomEventQueueTTL = 5 * time.Minute

// RoomEventQueuePush 将已序列化的事件帧追加到房间实时事件队列，并刷新 TTL。
// 调用方须先通过 proto.Marshal 将事件序列化为字节，再传入此函数。
func (c *Client) RoomEventQueuePush(ctx context.Context, roomID string, data []byte) error {
	key := RoomEventQueueKey(roomID)
	pipe := c.kv.Pipeline()
	pipe.RPush(ctx, key, data)
	pipe.Expire(ctx, key, roomEventQueueTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// RoomEventQueuePop 通过 BLPOP 长阻塞弹出一条事件字节序列；timeout 为 0 表示无限等待。
// 当 ctx 取消或 timeout 到期时返回 nil, nil。
// 调用方须通过 proto.Unmarshal 将返回字节反序列化为具体消息类型。
func (c *Client) RoomEventQueuePop(ctx context.Context, roomID string, timeout time.Duration) ([]byte, error) {
	key := RoomEventQueueKey(roomID)
	vals, err := c.kv.BLPop(ctx, timeout, key).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, nil // 超时，无数据
		}
		if ctx.Err() != nil {
			return nil, nil // ctx 已取消，正常退出
		}
		return nil, err
	}
	if len(vals) < 2 {
		return nil, nil
	}
	return []byte(vals[1]), nil
}
