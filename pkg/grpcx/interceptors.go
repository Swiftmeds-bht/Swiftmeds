package grpcx

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// healthPrefix marks health-check calls, which are not logged (Docker calls them every few seconds).
const healthPrefix = "/grpc.health.v1.Health/"

// LoggingUnary logs every unary call with its method, status code and duration.
func LoggingUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logCall(ctx, log, info.FullMethod, start, err)
		return resp, err
	}
}

// LoggingStream logs every streaming call when the stream ends.
func LoggingStream(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		logCall(ss.Context(), log, info.FullMethod, start, err)
		return err
	}
}

func logCall(ctx context.Context, log *slog.Logger, method string, start time.Time, err error) {
	if strings.HasPrefix(method, healthPrefix) {
		return
	}
	code := status.Code(err)
	attrs := []slog.Attr{
		slog.String("method", method),
		slog.String("code", code.String()),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	log.LogAttrs(ctx, levelFor(code), "grpc call", attrs...)
}

// levelFor logs server faults as errors, client mistakes (bad input, not found) as warnings.
func levelFor(code codes.Code) slog.Level {
	switch code {
	case codes.OK:
		return slog.LevelInfo
	case codes.Internal, codes.Unknown, codes.DataLoss, codes.Unavailable, codes.DeadlineExceeded, codes.Unimplemented:
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// RecoveryUnary turns a panic in a handler into codes.Internal instead of crashing the service.
func RecoveryUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recovered(ctx, log, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// RecoveryStream is RecoveryUnary for streaming calls.
func RecoveryStream(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recovered(ss.Context(), log, info.FullMethod, r)
			}
		}()
		return handler(srv, ss)
	}
}

func recovered(ctx context.Context, log *slog.Logger, method string, r any) error {
	log.LogAttrs(ctx, slog.LevelError, "panic in grpc handler",
		slog.String("method", method),
		slog.String("panic", fmt.Sprint(r)),
		slog.String("stack", string(debug.Stack())),
	)
	// Never send panic details to the caller.
	return status.Error(codes.Internal, "internal error")
}
