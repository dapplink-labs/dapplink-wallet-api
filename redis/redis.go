package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Address  string
	Password string
	DB       int
}

type Client struct {
	rdb *redis.Client
}

func New(cfg Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     50,
		MinIdleConns: 10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.rdb.Incr(ctx, key).Result()
}

func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *Client) HSet(ctx context.Context, key string, values ...interface{}) error {
	return c.rdb.HSet(ctx, key, values...).Err()
}

func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.rdb.HGetAll(ctx, key).Result()
}

func (c *Client) ZAdd(ctx context.Context, key string, score float64, member any) error {
	return c.rdb.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: member,
	}).Err()
}

func (c *Client) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.rdb.ZRevRange(ctx, key, start, stop).Result()
}

func (c *Client) TryLock(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, expiration).Result()
}

func (c *Client) Unlock(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

func (c *Client) Pipeline() redis.Pipeliner {
	return c.rdb.Pipeline()
}
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.rdb.Publish(ctx, channel, message).Err()
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.rdb.Subscribe(ctx, channels...)
}
