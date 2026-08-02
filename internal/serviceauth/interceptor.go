package serviceauth

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	// CallerServiceKey context key for caller service name
	CallerServiceKey contextKey = "caller_service"
	// CallerUserKey context key for original user ID
	CallerUserKey contextKey = "caller_user"
	// CallerEmailKey contextKey for original user email
	CallerEmailKey contextKey = "caller_email"
)

// UnaryClientInterceptor 返回 gRPC 客户端一元拦截器
// 自动签发服务间 JWT 并放入 gRPC metadata
func UnaryClientInterceptor(cfg *Config, logger *zap.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		// 从 context 中提取原始用户信息（如果有的话）
		var callerUser, callerEmail string
		if v := ctx.Value(CallerUserKey); v != nil {
			callerUser, _ = v.(string)
		}
		if v := ctx.Value(CallerEmailKey); v != nil {
			callerEmail, _ = v.(string)
		}

		// 签发服务间 JWT
		token, err := GenerateToken(cfg, callerUser, callerEmail)
		if err != nil {
			logger.Error("failed to generate service token",
				zap.String("method", method),
				zap.Error(err),
			)
			return status.Error(codes.Internal, "failed to generate service token")
		}

		// 将 token 放入 gRPC metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		// 使用新的 metadata 避免修改原始 context
		newMD := md.Copy()
		newMD.Set("authorization", "Bearer "+token)
		ctx = metadata.NewOutgoingContext(ctx, newMD)

		logger.Debug("service auth token attached",
			zap.String("method", method),
			zap.String("caller", cfg.ServiceName),
			zap.String("target", cfg.TargetService),
		)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerInterceptor 返回 gRPC 服务端一元拦截器
// 从 gRPC metadata 提取并验证服务间 JWT
func UnaryServerInterceptor(cfg *Config, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// 从 metadata 中提取 Authorization
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization in metadata")
		}

		authHeader := values[0]
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
		}

		// 验证服务间 JWT
		claims, err := ValidateToken(cfg, parts[1])
		if err != nil {
			logger.Warn("service auth failed",
				zap.String("method", info.FullMethod),
				zap.Error(err),
			)
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}

		// 将调用方信息注入 context
		ctx = context.WithValue(ctx, CallerServiceKey, claims.Sub)
		if claims.CallerUser != "" {
			ctx = context.WithValue(ctx, CallerUserKey, claims.CallerUser)
		}
		if claims.CallerEmail != "" {
			ctx = context.WithValue(ctx, CallerEmailKey, claims.CallerEmail)
		}

		logger.Debug("service auth passed",
			zap.String("method", info.FullMethod),
			zap.String("caller", claims.Sub),
			zap.String("caller_user", claims.CallerUser),
		)

		return handler(ctx, req)
	}
}
