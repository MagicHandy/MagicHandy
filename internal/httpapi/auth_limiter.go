package httpapi

import (
	"sync"
	"time"
)

const maxLoginLimiterKeys = 2048

type loginBucket struct {
	tokens float64
	last   time.Time
}

type loginLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	byIP      map[string]loginBucket
	byUser    map[string]loginBucket
	ipRate    float64
	ipBurst   float64
	userRate  float64
	userBurst float64
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		now:       func() time.Time { return time.Now().UTC() },
		byIP:      make(map[string]loginBucket),
		byUser:    make(map[string]loginBucket),
		ipRate:    30.0 / (5 * 60),
		ipBurst:   30,
		userRate:  8.0 / (5 * 60),
		userBurst: 8,
	}
}

// Allow consumes independent per-address and per-username tokens. Both must be
// available: combining them into one key would let one source sweep unlimited
// usernames or a distributed source hammer one known user.
func (l *loginLimiter) Allow(address, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if address == "" {
		address = "unknown"
	}
	if username == "" {
		username = "unknown"
	}
	now := l.now()
	if _, exists := l.byIP[address]; !exists && len(l.byIP)+len(l.byUser) >= maxLoginLimiterKeys {
		l.pruneLocked(now)
		if len(l.byIP)+len(l.byUser) >= maxLoginLimiterKeys {
			return false
		}
	}
	if !consumeLoginToken(l.byIP, address, now, l.ipRate, l.ipBurst) {
		return false
	}
	if _, exists := l.byUser[username]; !exists && len(l.byIP)+len(l.byUser) >= maxLoginLimiterKeys {
		l.pruneLocked(now)
		if len(l.byIP)+len(l.byUser) >= maxLoginLimiterKeys {
			return false
		}
	}
	if !consumeLoginToken(l.byUser, username, now, l.userRate, l.userBurst) {
		return false
	}
	return true
}

func consumeLoginToken(buckets map[string]loginBucket, key string, now time.Time, rate, burst float64) bool {
	if key == "" {
		key = "unknown"
	}
	bucket, ok := buckets[key]
	if !ok {
		bucket = loginBucket{tokens: burst, last: now}
	}
	if now.After(bucket.last) {
		bucket.tokens += now.Sub(bucket.last).Seconds() * rate
		if bucket.tokens > burst {
			bucket.tokens = burst
		}
		bucket.last = now
	}
	if bucket.tokens < 1 {
		buckets[key] = bucket
		return false
	}
	bucket.tokens--
	buckets[key] = bucket
	return true
}

func (l *loginLimiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-30 * time.Minute)
	for key, bucket := range l.byIP {
		if bucket.last.Before(cutoff) {
			delete(l.byIP, key)
		}
	}
	for key, bucket := range l.byUser {
		if bucket.last.Before(cutoff) {
			delete(l.byUser, key)
		}
	}
}
