package cachekit

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrLoad_CacheHit(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()

	data := map[string]int{"x": 1}
	bytes, _ := json.Marshal(data)
	mock.ExpectGet("key").SetVal(string(bytes))

	loadCalled := false
	val, err := GetOrLoad(c, ctx, "key", time.Minute, func(context.Context) (map[string]int, error) {
		loadCalled = true
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, data, val)
	assert.False(t, loadCalled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_CacheMiss(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()

	mock.ExpectGet("key").SetErr(redis.Nil)
	mock.ExpectSet("key", []byte(`{"x":2}`), time.Minute).SetVal("OK")

	val, err := GetOrLoad(c, ctx, "key", time.Minute, func(context.Context) (map[string]int, error) {
		return map[string]int{"x": 2}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"x": 2}, val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDel(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()

	mock.ExpectDel("k1", "k2").SetVal(2)
	err := c.Del(ctx, "k1", "k2")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSet(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()

	mock.ExpectSet("k", []byte(`{"N":10}`), time.Second).SetVal("OK")
	err := c.Set(ctx, "k", struct{ N int }{10}, time.Second)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteByPrefix(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()

	mock.ExpectScan(0, "p*", 500).SetVal([]string{"p1", "p2"}, 0)
	mock.ExpectUnlink("p1", "p2").SetVal(2)
	err := c.DeleteByPrefix(ctx, "p")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_EmptyKey(t *testing.T) {
	t.Parallel()
	client, _ := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	_, err := GetOrLoad(c, ctx, "", time.Minute, func(context.Context) (int, error) { return 0, nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestGetOrLoad_ZeroTTL(t *testing.T) {
	t.Parallel()
	client, _ := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	_, err := GetOrLoad(c, ctx, "k", 0, func(context.Context) (int, error) { return 0, nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ttl")
}

func TestGetOrLoad_LoadError(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectGet("k").SetErr(redis.Nil)
	c := New(client)
	ctx := context.Background()
	loadErr := errors.New("load failed")
	_, err := GetOrLoad(c, ctx, "k", time.Minute, func(context.Context) (int, error) {
		return 0, loadErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, loadErr)
}

func TestGetOrLoad_RedisSetError(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectGet("k").SetErr(redis.Nil)
	mock.ExpectSet("k", []byte(`42`), time.Minute).SetErr(errors.New("redis set failed"))
	c := New(client)
	ctx := context.Background()
	val, err := GetOrLoad(c, ctx, "k", time.Minute, func(context.Context) (int, error) { return 42, nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set after load")
	assert.Equal(t, 42, val)
}

func TestGetOrLoad_SingleflightDedup(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectGet("k").SetErr(redis.Nil)
	mock.ExpectSet("k", []byte(`1`), time.Minute).SetVal("OK")
	mock.ExpectGet("k").SetVal("1")
	c := New(client)
	ctx := context.Background()
	var loadCalls atomic.Int32
	loadFn := func(context.Context) (int, error) {
		loadCalls.Add(1)
		return 1, nil
	}
	v1, err := GetOrLoad(c, ctx, "k", time.Minute, loadFn)
	require.NoError(t, err)
	assert.Equal(t, 1, v1)
	assert.Equal(t, int32(1), loadCalls.Load())
	v2, err := GetOrLoad(c, ctx, "k", time.Minute, loadFn)
	require.NoError(t, err)
	assert.Equal(t, 1, v2)
	assert.Equal(t, int32(1), loadCalls.Load())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOrLoad_UnmarshalError(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectGet("k").SetVal("not valid json")
	mock.ExpectDel("k").SetVal(1)
	c := New(client)
	ctx := context.Background()
	_, err := GetOrLoad(c, ctx, "k", time.Minute, func(context.Context) (map[string]int, error) {
		return nil, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestSet_EmptyKey(t *testing.T) {
	t.Parallel()
	client, _ := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	err := c.Set(ctx, "", 1, time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestSet_ZeroTTL(t *testing.T) {
	t.Parallel()
	client, _ := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	err := c.Set(ctx, "k", 1, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ttl")
}

func TestDeleteByPrefix_EmptyPrefix(t *testing.T) {
	t.Parallel()
	client, _ := redismock.NewClientMock()
	c := New(client)
	ctx := context.Background()
	err := c.DeleteByPrefix(ctx, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prefix")
}

func TestDeleteByPrefix_MaxIterations(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectScan(0, "x*", 500).SetVal([]string{"x1"}, 1)
	mock.ExpectUnlink("x1").SetVal(1)
	mock.ExpectScan(1, "x*", 500).SetVal([]string{"x2"}, 2)
	mock.ExpectUnlink("x2").SetVal(1)
	mock.ExpectScan(2, "x*", 500).SetVal([]string{"x3"}, 0)
	mock.ExpectUnlink("x3").SetVal(1)
	c := New(client)
	ctx := context.Background()
	err := c.DeleteByPrefix(ctx, "x", 3)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
