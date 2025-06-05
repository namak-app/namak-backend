package model

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/permission/model"
	restaurantModel "github.com/KhoshMaze/khoshmaze-backend/internal/domain/restaurant/model"
	"golang.org/x/crypto/bcrypt"
)

type (
	UserID uint
	Phone  string
)

func (p Phone) IsValid() bool {
	re := regexp.MustCompile(`^\+989\d{9}$`)
	return re.MatchString(string(p))
}

func (u *User) Validate() error {
	if !u.Phone.IsValid() {
		return errors.New("invalid phone")
	}

	if u.FirstName == "" {
		return errors.ErrUnsupported
	}

	if u.LastName == "" {
		return errors.ErrUnsupported
	}

	return nil
}

type User struct {
	ID          UserID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FirstName   string
	LastName    string
	Phone       Phone
	Permissions model.UserPermissions
	Roles       model.UserRoles
	Restaurants []*restaurantModel.Restaurant
}

type TokenWhitelist struct {
	ExpiresAt time.Time
	Value     string
	UserID    UserID
}

func (t *TokenWhitelist) HashTokenValue() error{
	hash, err := bcrypt.GenerateFromPassword([]byte(t.Value), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	t.Value = base64.StdEncoding.EncodeToString(hash)
	return nil
}

type UserFilter struct {
	ID    UserID
	Phone string
}

func (f *UserFilter) IsValid() bool {
	f.Phone = strings.TrimSpace(f.Phone)
	return f.ID > 0 || len(f.Phone) > 0
}
