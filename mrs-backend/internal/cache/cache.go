package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache Redis 热点查询缓存（ICD 检索、统计报表）。
// Redis 不可用时自动降级为直查数据库，业务不受影响；骨架期用短 TTL，不做精确失效。
type Cache struct {
	rdb *redis.Client
	ctx context.Context
}

func New(addr, password string, db int) *Cache {
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	ctx := context.Background()
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		log.Printf("[cache] Redis 不可用(%v)，缓存降级为直查", err)
		return &Cache{rdb: nil, ctx: ctx}
	}
	log.Printf("[cache] Redis 已连接 %s", addr)
	return &Cache{rdb: rdb, ctx: ctx}
}

func (c *Cache) Enabled() bool { return c != nil && c.rdb != nil }

// GetJSON 命中返回 true 并把缓存值写入 out
func (c *Cache) GetJSON(key string, out any) bool {
	if !c.Enabled() {
		return false
	}
	val, err := c.rdb.Get(c.ctx, key).Result()
	if err != nil {
		return false
	}
	return json.Unmarshal([]byte(val), out) == nil
}

// SetJSON 写缓存，失败仅记日志
func (c *Cache) SetJSON(key string, val any, ttl time.Duration) {
	if !c.Enabled() {
		return
	}
	b, err := json.Marshal(val)
	if err != nil {
		return
	}
	if err := c.rdb.Set(c.ctx, key, b, ttl).Err(); err != nil {
		log.Printf("[cache] 写入失败 %s: %v", key, err)
	}
}
