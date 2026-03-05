package cluster_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"github.com/afterdarksys/apiproxyd/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClusterInvalidation verifies that a gRPC client can successfully request
// cache eviction on a running gRPC server node.
func TestClusterInvalidation(t *testing.T) {
	// Initialize a memory cache to act as Node A's cache store
	nodeA_Cache := cache.NewMemoryCache(100, 1*time.Hour, 10*time.Second)

	// Pre-populate some keys
	nodeA_Cache.Set("key-1", []byte("val1"))
	nodeA_Cache.Set("key-2", []byte("val2"))

	// Verify they exist
	val, err := nodeA_Cache.Get("key-1")
	require.NoError(t, err)
	assert.Equal(t, "val1", string(val))

	// Boot up Cluster Node A on port 9011
	nodeA := cluster.NewServer("node-a", nodeA_Cache)
	err = nodeA.Start(9011)
	require.NoError(t, err, "Node A should start successfully")
	defer nodeA.Stop()

	// Give the gRPC server a moment to bind
	time.Sleep(100 * time.Millisecond)

	// Spin up a mock client (representing Node B) to connect to Node A
	client, err := cluster.NewClient(fmt.Sprintf("127.0.0.1:%d", 9011))
	require.NoError(t, err, "Client should connect to Node A")
	defer client.Close()

	// Node B explicitly invalidates key-2 on Node A
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.Invalidate(ctx, "key-2", "node-b")
	require.NoError(t, err, "Invalidation request should succeed")

	// Verify key-2 is actually gone on Node A
	_, err = nodeA_Cache.Get("key-2")
	assert.Error(t, err, "Get for key-2 should return an error as it should be evicted")
	assert.Contains(t, err.Error(), "miss", "Error should indicate a cache miss")

	// Verify key-1 remains untouched
	val, err = nodeA_Cache.Get("key-1")
	require.NoError(t, err)
	assert.Equal(t, "val1", string(val), "key-1 should still exist")
}
