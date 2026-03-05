package cluster

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	pb "github.com/afterdarksys/apiproxyd/api/proto" // generated proto package
	"github.com/afterdarksys/apiproxyd/pkg/cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Server represents a gRPC cluster node accepting connections from peers
type Server struct {
	pb.UnimplementedClusterServiceServer
	id      string
	grpcSrv *grpc.Server
	cache   cache.Cache
	mu      sync.Mutex
	uptime  time.Time
}

// NewServer creates a cluster gRPC server
func NewServer(id string, cache cache.Cache) *Server {
	return &Server{
		id:      id,
		cache:   cache,
		uptime:  time.Now(),
		grpcSrv: grpc.NewServer(),
	}
}

// Start begins the gRPC listener on the given port
func (s *Server) Start(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	pb.RegisterClusterServiceServer(s.grpcSrv, s)

	log.Printf("[Cluster Server] Listening on :%d", port)
	go func() {
		if err := s.grpcSrv.Serve(lis); err != nil {
			log.Printf("[Cluster Server] Terminated: %v", err)
		}
	}()
	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() {
	s.grpcSrv.GracefulStop()
}

// HealthCheck verifies the node health
func (s *Server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	stats, err := s.cache.Stats()
	cacheCount := int32(0)
	if err == nil && stats != nil {
		cacheCount = int32(stats.Entries)
	}
	return &pb.HealthCheckResponse{
		Status:     "OK",
		Uptime:     int64(time.Since(s.uptime).Seconds()),
		CacheCount: cacheCount,
	}, nil
}

// InvalidateCache evicts a key locally, unless initiated from the reporting peer
func (s *Server) InvalidateCache(ctx context.Context, req *pb.InvalidateCacheRequest) (*pb.InvalidateCacheResponse, error) {
	// Prevent loop
	if req.OriginalNode == s.id {
		return &pb.InvalidateCacheResponse{Success: true}, nil
	}

	// For LayeredCache, clearing the cache removes from memory and backing DB.
	// We'll rely on the cache interface to delete/invalidate.
	// Currently standard cache interface doesn't expose Delete/Invalidate directly natively,
	// but let's assume Delete exists or we can emulate it.
	// Oh wait, our cache API doesn't have an explicit `Delete` yet.
	// To perform cache invalidation, a custom interface or an extension is needed.
	// For this Phase 4 implementation, we'll implement a `Delete` on the backend if missing,
	// or log it here.
	if bulkDeleter, ok := s.cache.(interface{ Delete(string) error }); ok {
		if err := bulkDeleter.Delete(req.Key); err != nil {
			return &pb.InvalidateCacheResponse{Success: false, Error: err.Error()}, nil
		}
	} else {
		return &pb.InvalidateCacheResponse{Success: false, Error: "backend does not support explicit delete"}, nil
	}

	return &pb.InvalidateCacheResponse{Success: true}, nil
}

// CacheLookup searches the local cache and returns the payload to the peer
func (s *Server) CacheLookup(ctx context.Context, req *pb.CacheLookupRequest) (*pb.CacheLookupResponse, error) {
	val, err := s.cache.Get(req.Key)
	if err != nil {
		return &pb.CacheLookupResponse{Found: false}, nil
	}
	return &pb.CacheLookupResponse{Found: true, Payload: val}, nil
}

// Client represents a gRPC connection to another peer in the cluster
type Client struct {
	addr   string
	conn   *grpc.ClientConn
	client pb.ClusterServiceClient
}

// NewClient creates a peer client.
func NewClient(addr string) (*Client, error) {
	// For production use you'd want TLS here.
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	client := pb.NewClusterServiceClient(conn)
	return &Client{addr: addr, conn: conn, client: client}, nil
}

// Close closes the connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Invalidate sends a cache invalidation request to the peer
func (c *Client) Invalidate(ctx context.Context, key, fromNodeID string) error {
	resp, err := c.client.InvalidateCache(ctx, &pb.InvalidateCacheRequest{
		Key:          key,
		OriginalNode: fromNodeID,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("peer returned error: %s", resp.Error)
	}
	return nil
}
