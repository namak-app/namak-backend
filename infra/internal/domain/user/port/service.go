package port

import (
	"context"

	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/model"
)

type Service interface {
	CreateUser(ctx context.Context, user model.User) (model.UserID, error)
	GetUserByFilter(ctx context.Context, filter *model.UserFilter) (*model.User, error)
	IsValidToken(ctx context.Context, value string) bool
	CreateToken(ctx context.Context, token model.TokenWhitelist) error
	DeleteToken(ctx context.Context, token model.TokenWhitelist) error
	DeleteAllTokens(ctx context.Context, id model.UserID) error
}
