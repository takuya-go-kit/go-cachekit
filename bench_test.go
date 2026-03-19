package cachekit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

func BenchmarkBoundedCache_Set(b *testing.B) {
	c := NewBoundedCache[string, int](1024)
	for i := 0; i < b.N; i++ {
		c.Set(strconv.Itoa(i%2048), i)
	}
}

func BenchmarkBoundedCache_Get_Hit(b *testing.B) {
	c := NewBoundedCache[string, int](1024)
	for i := 0; i < 1024; i++ {
		c.Set(strconv.Itoa(i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(strconv.Itoa(i % 1024))
	}
}

func BenchmarkBoundedCache_Get_Miss(b *testing.B) {
	c := NewBoundedCache[string, int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("miss")
	}
}

func BenchmarkBoundedCache_SetEviction(b *testing.B) {
	c := NewBoundedCache[int, int](256)
	for i := 0; i < 256; i++ {
		c.Set(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(256+i, i)
	}
}

func BenchmarkBoundedCache_Parallel_Get(b *testing.B) {
	c := NewBoundedCache[int, int](1024)
	for i := 0; i < 1024; i++ {
		c.Set(i, i)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(i % 1024)
			i++
		}
	})
}

func BenchmarkBoundedCache_Parallel_SetGet(b *testing.B) {
	c := NewBoundedCache[int, int](1024)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set(i%1024, i)
			} else {
				c.Get(i % 1024)
			}
			i++
		}
	})
}

func BenchmarkBoundedCache_Delete(b *testing.B) {
	c := NewBoundedCache[int, int](4096)
	for i := 0; i < 4096; i++ {
		c.Set(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Delete(i % 4096)
	}
}

func BenchmarkAddRemoveInFlight(b *testing.B) {
	client, _ := redismock.NewClientMock()
	c := New(client)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addInFlight(c, "key")
		removeInFlight(c, "key")
	}
}

func BenchmarkAddRemoveInFlight_Parallel(b *testing.B) {
	client, _ := redismock.NewClientMock()
	c := New(client)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			addInFlight(c, "key")
			removeInFlight(c, "key")
		}
	})
}

func BenchmarkCacheKeyVersion(b *testing.B) {
	client, _ := redismock.NewClientMock()
	c := New(client)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cacheKeyVersion(c, "key")
	}
}

func BenchmarkGetOrLoad_CacheHit(b *testing.B) {
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	data := map[string]int{"x": 1}
	bytes, _ := json.Marshal(data)
	for i := 0; i < b.N; i++ {
		mock.ExpectGet("key").SetVal(string(bytes))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetOrLoad(c, ctx, "key", time.Minute, func(context.Context) (map[string]int, error) {
			return nil, nil
		})
	}
}

func BenchmarkGetOrLoad_CacheMiss(b *testing.B) {
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		mock.ExpectGet(key).SetErr(redis.Nil)
		mock.ExpectSet(key, []byte(`{"x":1}`), time.Minute).SetVal("OK")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, _ = GetOrLoad(c, ctx, key, time.Minute, func(context.Context) (map[string]int, error) {
			return map[string]int{"x": 1}, nil
		})
	}
}

func BenchmarkCacheSet(b *testing.B) {
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		mock.ExpectSet("key", []byte(`42`), time.Minute).SetVal("OK")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, "key", 42, time.Minute)
	}
}

func BenchmarkCacheDel(b *testing.B) {
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		mock.ExpectDel("key").SetVal(1)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Del(ctx, "key")
	}
}

func BenchmarkCachedValue_Get_Hit(b *testing.B) {
	v := NewCachedValue[int](context.Background(), "k", time.Minute)
	defer v.Stop()
	ctx := context.Background()
	_, _ = v.Get(ctx, func(context.Context) (int, error) { return 42, nil })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.Get(ctx, func(context.Context) (int, error) { return 42, nil })
	}
}

func BenchmarkCachedValue_Get_Miss(b *testing.B) {
	v := NewCachedValue[int](context.Background(), "k", time.Nanosecond)
	defer v.Stop()
	ctx := context.Background()
	time.Sleep(5 * time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Invalidate()
		_, _ = v.Get(ctx, func(context.Context) (int, error) { return i, nil })
	}
}

func BenchmarkCachedValue_Get_Parallel(b *testing.B) {
	v := NewCachedValue[int](context.Background(), "k", time.Minute)
	defer v.Stop()
	ctx := context.Background()
	_, _ = v.Get(ctx, func(context.Context) (int, error) { return 42, nil })
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = v.Get(ctx, func(context.Context) (int, error) { return 42, nil })
		}
	})
}

func BenchmarkCachedValue_Invalidate(b *testing.B) {
	v := NewCachedValue[int](context.Background(), "k", time.Minute)
	defer v.Stop()
	ctx := context.Background()
	_, _ = v.Get(ctx, func(context.Context) (int, error) { return 42, nil })
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Invalidate()
	}
}

func BenchmarkEvictVersionMapExcess(b *testing.B) {
	client, _ := redismock.NewClientMock()
	c := New(client, WithMaxVersionMapEntries(100))
	for i := 0; i < 200; i++ {
		cacheKeyVersion(c, fmt.Sprintf("key-%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.evictMu.Lock()
		evictVersionMapExcess(c, "key-0")
		c.evictMu.Unlock()
	}
}

func BenchmarkEscapeRedisGlob(b *testing.B) {
	for i := 0; i < b.N; i++ {
		escapeRedisGlob("user:*:profile[1]?")
	}
}

func BenchmarkBoundedCache_Parallel_HeavyContention(b *testing.B) {
	c := NewBoundedCache[int, int](64)
	var wg sync.WaitGroup
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()
		i := 0
		for pb.Next() {
			switch i % 10 {
			case 0, 1:
				c.Set(i%64, i)
			case 2:
				c.Delete(i % 64)
			default:
				c.Get(i % 64)
			}
			i++
		}
	})
	wg.Wait()
}
