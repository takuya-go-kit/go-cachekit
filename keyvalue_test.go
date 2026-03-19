package cachekit

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisKeyValueStore_NilClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := &RedisKeyValueStore{}
	val, err := s.Get(ctx, "k")
	assert.ErrorIs(t, err, ErrRedisNotConfigured)
	assert.Empty(t, val)

	err = s.Set(ctx, "k", []byte("v"), time.Minute)
	assert.ErrorIs(t, err, ErrRedisNotConfigured)

	err = s.Del(ctx, "k")
	assert.ErrorIs(t, err, ErrRedisNotConfigured)
}

func TestRedisKeyValueStore_NilReceiver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var s *RedisKeyValueStore
	_, err := s.Get(ctx, "k")
	assert.ErrorIs(t, err, ErrRedisNotConfigured)
	err = s.Set(ctx, "k", []byte("v"), time.Minute)
	assert.ErrorIs(t, err, ErrRedisNotConfigured)
	err = s.Del(ctx, "k")
	assert.ErrorIs(t, err, ErrRedisNotConfigured)
}

func TestRedisKeyValueStore_Get_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	mock.ExpectGet("missing").SetErr(redis.Nil)
	store := &RedisKeyValueStore{Client: client}
	val, err := store.Get(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRedisKeyValueStore_SetGet_RoundTrip(t *testing.T) {
	t.Parallel()
	client, mock := redismock.NewClientMock()
	payload := []byte(`{"x":1}`)
	mock.ExpectSet("k", payload, time.Minute).SetVal("OK")
	mock.ExpectGet("k").SetVal(string(payload))
	store := &RedisKeyValueStore{Client: client}
	ctx := context.Background()
	err := store.Set(ctx, "k", payload, time.Minute)
	require.NoError(t, err)
	got, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
