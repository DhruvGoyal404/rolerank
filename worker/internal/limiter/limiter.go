// Package limiter spaces requests to a host across every worker. The schedule lives
// in Valkey and uses Valkey's clock, so adding workers can never raise the rate.
package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// reserve takes the next free slot for a host and returns how long to wait for it.
var reserve = valkey.NewLuaScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local slot = tonumber(redis.call('GET', KEYS[1]) or '0')
if slot < now then slot = now end
redis.call('SET', KEYS[1], slot + ARGV[1], 'PX', ARGV[1] * 2 + 60000)
return slot - now
`)

// block pushes a host's next slot out, for Retry-After and backoff.
var block = valkey.NewLuaScript(`
local t = redis.call('TIME')
local now = t[1] * 1000 + math.floor(t[2] / 1000)
local slot = tonumber(redis.call('GET', KEYS[1]) or '0')
local until_ms = now + ARGV[1]
if until_ms > slot then
  redis.call('SET', KEYS[1], until_ms, 'PX', ARGV[1] + 60000)
end
return 0
`)

// Limiter is shared by all workers through one Valkey.
type Limiter struct {
	client valkey.Client
	prefix string
}

// New returns a limiter. Prefix keeps test runs apart.
func New(client valkey.Client, prefix string) *Limiter {
	return &Limiter{client: client, prefix: prefix}
}

func (l *Limiter) key(host string) string {
	return l.prefix + "host:" + host
}

// Wait blocks until this caller's slot for host, at most one per interval.
func (l *Limiter) Wait(ctx context.Context, host string, interval time.Duration) error {
	ms := interval.Milliseconds()
	if ms < 1 {
		return fmt.Errorf("limiter: interval %v too small", interval)
	}
	wait, err := reserve.Exec(ctx, l.client, []string{l.key(host)}, []string{fmt.Sprint(ms)}).AsInt64()
	if err != nil {
		return fmt.Errorf("limiter: reserve: %w", err)
	}
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(wait) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Block stops all workers calling host for d.
func (l *Limiter) Block(ctx context.Context, host string, d time.Duration) error {
	ms := d.Milliseconds()
	if ms < 1 {
		return nil
	}
	err := block.Exec(ctx, l.client, []string{l.key(host)}, []string{fmt.Sprint(ms)}).Error()
	if err != nil && !valkey.IsValkeyNil(err) {
		return fmt.Errorf("limiter: block: %w", err)
	}
	return nil
}
