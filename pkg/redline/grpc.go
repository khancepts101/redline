package redline

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (r *Redline) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		r.rate.Add(1)
		ctx = withLogger(ctx, r.cfg.Logger)
		defer func() {
			v := recover()
			if v != nil {
				r.capture(ctx, v)
				err = status.Error(codes.Internal, "internal server error")
			}
			// Record before any re-panic so the request is still accounted for.
			r.recordRPC(ctx, "unary", info.FullMethod, start, err)
			if v != nil && r.cfg.PanicMode == PanicRepanic {
				panic(v)
			}
		}()
		return handler(ctx, req)
	}
}
func (r *Redline) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		start := time.Now()
		r.rate.Add(1)
		defer func() {
			v := recover()
			if v != nil {
				r.capture(ss.Context(), v)
				err = status.Error(codes.Internal, "internal server error")
			}
			// Record before any re-panic so the request is still accounted for.
			r.recordRPC(ss.Context(), "stream", info.FullMethod, start, err)
			if v != nil && r.cfg.PanicMode == PanicRepanic {
				panic(v)
			}
		}()
		return handler(srv, ss)
	}
}
func (r *Redline) recordRPC(ctx context.Context, transport, method string, start time.Time, err error) {
	after := time.Now()
	code := status.Code(err).String()
	r.requests.WithLabelValues(r.cfg.Service, transport, method, method, code).Inc()
	if err != nil {
		r.errors.WithLabelValues(r.cfg.Service, transport, method, method, code).Inc()
	}
	r.observe(r.duration.WithLabelValues(r.cfg.Service, transport, method, method), after.Sub(start).Seconds(), ctx)
	r.overhead.WithLabelValues(r.cfg.Service, transport).Observe(time.Since(after).Seconds())
}

var _ prometheus.Observer
