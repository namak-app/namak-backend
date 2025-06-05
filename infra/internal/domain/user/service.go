package user

import (
	"context"
	"errors"

	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/model"
	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/port"
)

var (
	ErrUserOnCreate           = errors.New("error on creating new user")
	ErrUserCreationValidation = errors.New("validation failed")
	ErrUserNotFound           = errors.New("user not found")
	ErrTokenHash              = errors.New("error on hashing token value")
)

type service struct {
	repo port.Repo
}

func NewService(repo port.Repo) port.Service {
	return &service{
		repo: repo,
	}
}

func (s *service) CreateUser(ctx context.Context, user model.User) (model.UserID, error) {
	if err := user.Validate(); err != nil {
		return 0, ErrUserOnCreate
	}
	userID, err := s.repo.Create(ctx, user)
	return userID, err
}

func (s *service) IsValidToken(ctx context.Context, value string) bool {
	// Fix this later and hash the values without creating a struct
	tk := &model.TokenWhitelist{Value: value}
	tk.HashValue()
	err := s.repo.IsValidToken(ctx, tk.Value)
	if err != nil {
		return false
	}
	return true
}

func (s *service) CreateToken(ctx context.Context, token model.TokenWhitelist) error {
	token.HashValue()
	return s.repo.CreateToken(ctx, token)
}

func (s *service) DeleteToken(ctx context.Context, token model.TokenWhitelist) error {
	token.HashValue()
	return s.repo.DeleteToken(ctx, token)
}

func (s *service) DeleteAllTokens(ctx context.Context, id model.UserID) error {
	return s.repo.DeleteAllTokens(ctx, id)
}

func (s *service) GetUserByFilter(ctx context.Context, filter *model.UserFilter) (*model.User, error) {
	user, err := s.repo.GetByFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}
