// Package grpcapi is the gRPC adapter: it exposes CreateUser and GetUser
// over gRPC, reusing the same user use case as HTTP. Authentication uses
// the same JWTs, sent as "authorization: Bearer <token>" metadata and
// enforced by a unary interceptor.
package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	userv1 "github.com/minearithmeticop/thaimart-backend-challenge/api/proto/user/v1"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app/user"
	"github.com/minearithmeticop/thaimart-backend-challenge/internal/domain"
)

// Server implements the generated UserServiceServer by delegating to the
// user use case.
type Server struct {
	userv1.UnimplementedUserServiceServer
	users  *user.Service
	tokens app.TokenManager
	log    *slog.Logger
}

// NewServer builds the gRPC server with auth + logging interceptors and
// server reflection (so grpcurl works out of the box).
func NewServer(users *user.Service, tokens app.TokenManager, log *slog.Logger) *grpc.Server {
	srv := &Server{users: users, tokens: tokens, log: log}

	grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		unaryAuthInterceptor(tokens),
		unaryLoggingInterceptor(log),
	))
	userv1.RegisterUserServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv)
	return grpcSrv
}

// unaryAuthInterceptor verifies the JWT carried in "authorization"
// metadata. Reflection stays public so tooling can discover the service.
func unaryAuthInterceptor(tokens app.TokenManager) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		values := md.Get("authorization")
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, `missing "authorization" metadata, expected: Bearer <token>`)
		}
		token, found := strings.CutPrefix(values[0], "Bearer ")
		if !found || token == "" {
			return nil, status.Error(codes.Unauthenticated, `malformed "authorization" metadata, expected: Bearer <token>`)
		}

		if _, err := tokens.Verify(strings.TrimSpace(token)); err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return handler(ctx, req)
	}
}

// unaryLoggingInterceptor logs one line per RPC.
func unaryLoggingInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			log.Warn("grpc request", "method", info.FullMethod, "code", status.Code(err).String())
		} else {
			log.Info("grpc request", "method", info.FullMethod, "code", codes.OK.String())
		}
		return resp, err
	}
}

// CreateUser implements user.v1.UserService.CreateUser.
func (s *Server) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	u, err := s.users.Create(ctx, req.GetName(), req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.CreateUserResponse{User: toProto(u)}, nil
}

// GetUser implements user.v1.UserService.GetUser.
func (s *Server) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	u, err := s.users.Get(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &userv1.GetUserResponse{User: toProto(u)}, nil
}

func toProto(u domain.User) *userv1.User {
	return &userv1.User{
		Id:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: timestamppb.New(u.CreatedAt),
	}
}

// toStatus maps domain sentinel errors onto gRPC status codes — the gRPC
// twin of httpapi.domainStatus.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidInput), errors.Is(err, domain.ErrInvalidUserID):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, "email already exists")
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, "unauthorized")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
