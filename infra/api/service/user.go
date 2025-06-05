package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/KhoshMaze/khoshmaze-backend/api/pb"
	"github.com/KhoshMaze/khoshmaze-backend/api/utils"
	"github.com/KhoshMaze/khoshmaze-backend/internal/adapters/jwt"
	timeutils "github.com/KhoshMaze/khoshmaze-backend/internal/adapters/time"
	notifDomain "github.com/KhoshMaze/khoshmaze-backend/internal/domain/notification/model"
	notifPort "github.com/KhoshMaze/khoshmaze-backend/internal/domain/notification/port"
	perm "github.com/KhoshMaze/khoshmaze-backend/internal/domain/permission/model"
	"github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/model"
	userPort "github.com/KhoshMaze/khoshmaze-backend/internal/domain/user/port"
	jwt5 "github.com/golang-jwt/jwt/v5"
)

var (
	ErrWrongOTP            = errors.New("wrong otp")
	ErrOTPAlreadySent      = errors.New("otp already sent. wait 2 minutes before sending again")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrUserOnCreate        = errors.New("couldn't create the user")
	ErrUserNotFound        = errors.New("user not found")
)

type UserService struct {
	svc           userPort.Service
	notifSvc      notifPort.Service
	authSecret    string
	refreshSecret string
	aesSecret     string
	expMin        uint
	refreshExpMin uint
}

func NewUserService(svc userPort.Service, authSecret, refreshSecret, aesSecret string, expMin, refreshExpMin uint, notifSvc notifPort.Service) *UserService {
	return &UserService{
		svc:           svc,
		notifSvc:      notifSvc,
		authSecret:    authSecret,
		refreshSecret: refreshSecret,
		aesSecret:     aesSecret,
		expMin:        expMin,
		refreshExpMin: refreshExpMin,
	}
}

func (s *UserService) SignUp(ctx context.Context, req *pb.UserSignUpRequest) (*pb.UserTokenResponse, error) {
	ok, err := s.notifSvc.CheckUserNotifValue(ctx, model.Phone(req.GetPhone()), req.GetOtp())
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrWrongOTP
	}

	if user, _ := s.svc.GetUserByFilter(ctx, &model.UserFilter{
		Phone: req.GetPhone(),
	}); user != nil {
		return nil, ErrUserAlreadyExists
	}

	user := model.User{
		FirstName:   req.GetFirstName(),
		LastName:    req.GetLastName(),
		Phone:       model.Phone(req.GetPhone()),
		Permissions: perm.Read + perm.Create + perm.Update + perm.Delete,
	}

	userId, err := s.svc.CreateUser(ctx, user)

	if err != nil {
		return nil, ErrUserOnCreate
	}

	s.notifSvc.DeleteUserNotifValue(ctx, model.Phone(req.GetPhone()))
	return s.generateTokenResponse(ctx, &jwt.UserClaims{UserID: uint(userId),
		Phone:       req.GetPhone(),
		Permissions: uint64(user.Permissions),
		Roles:       uint64(perm.RestaurantOwner)},
		true)

}

func (s *UserService) Login(ctx context.Context, req *pb.UserLoginRequest) (*pb.UserTokenResponse, error) {
	ok, err := s.notifSvc.CheckUserNotifValue(ctx, model.Phone(req.GetPhone()), req.GetOtp())
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, ErrWrongOTP
	}

	user, err := s.svc.GetUserByFilter(ctx, &model.UserFilter{
		Phone: req.GetPhone(),
	})

	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}

	s.notifSvc.DeleteUserNotifValue(ctx, model.Phone(req.GetPhone()))
	return s.generateTokenResponse(ctx, &jwt.UserClaims{UserID: uint(user.ID),
		Phone:       string(user.Phone),
		Permissions: uint64(user.Permissions),
		Roles:       uint64(user.Roles)}, true)

}

func (s *UserService) SendOTP(ctx context.Context, req *pb.OtpRequest) (string, error) {
	var (
		phone = req.GetPhone()
	)

	if notif, _ := s.notifSvc.GetUserNotif(ctx, model.Phone(phone)); notif != "" {
		return "", ErrOTPAlreadySent
	}
	user, err := s.svc.GetUserByFilter(ctx, &model.UserFilter{
		Phone: phone,
	})

	code := rand.IntN(999999) + 100000
	notif := notifDomain.NewNotification(0, fmt.Sprint(code), notifDomain.NotifTypeSMS, true, time.Second*150, model.Phone(phone))
	err = s.notifSvc.Send(ctx, notif)

	if !notif.ForAuthorization {
		if err != nil {
			return "", err
		}
	}

	if user != nil {
		return "login", nil
	}

	return "register", nil
}

func (s *UserService) Logout(ctx context.Context, token string) error {
	userClaims, err := utils.UserClaimsFromCookies(token, []byte(s.refreshSecret))

	if err != nil {
		return err
	}

	if ok := !s.svc.IsValidToken(ctx, token); ok {
		return ErrInvalidRefreshToken
	}

	err = s.svc.DeleteToken(ctx, model.TokenWhitelist{
		Value:     token,
		ExpiresAt: userClaims.ExpiresAt.Time,
		UserID:    model.UserID(userClaims.UserID),
	})

	return err
}

func (s *UserService) RefreshToken(ctx context.Context, token string) (*pb.UserTokenResponse, error) {
	userClaims, err := utils.UserClaimsFromCookies(token, []byte(s.refreshSecret))

	if err != nil {
		return nil, err
	}

	if ok := !s.svc.IsValidToken(ctx, token); ok {
		return nil, ErrInvalidRefreshToken
	}

	user, err := s.svc.GetUserByFilter(ctx, &model.UserFilter{
		ID: model.UserID(userClaims.UserID),
	})

	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}

	userClaims.Permissions = uint64(user.Permissions)
	userClaims.Roles = uint64(user.Roles)
	userClaims.Phone = string(user.Phone)

	// TODO: DELETE THE TOKEN WITHOUT REPLACING?
	if time.Until(userClaims.ExpiresAt.Time)/time.Hour < 12 {

		err = s.svc.DeleteToken(ctx, model.TokenWhitelist{
			Value:     token,
			ExpiresAt: userClaims.ExpiresAt.Time,
			UserID:    model.UserID(userClaims.UserID),
		})

		if err != nil {
			return nil, err
		}
		return s.generateTokenResponse(ctx, userClaims, true)
	}

	return s.generateTokenResponse(ctx, userClaims, false)
}

func (s *UserService) createToken(claims *jwt.UserClaims, isRefresh bool) (string, int64, error) {
	var (
		secret string = s.authSecret
		exp    uint   = s.expMin
	)

	if isRefresh {
		secret = s.refreshSecret
		exp = s.refreshExpMin
	}

	claims.ExpiresAt = jwt5.NewNumericDate(timeutils.AddMinutes(exp, true))
	token, err := jwt.CreateToken([]byte(secret), claims)

	if err != nil {
		return "", 0, err
	}

	return token, int64(exp * 60), nil
}

func (s *UserService) generateTokenResponse(ctx context.Context, claims *jwt.UserClaims, genRefresh bool) (*pb.UserTokenResponse, error) {
	cp := *claims
	access, accessMaxAge, err := s.createToken(claims, false)
	if err != nil {
		return nil, err
	}

	var (
		refresh       string
		refreshMaxAge int64
	)
	if genRefresh {

		refresh, refreshMaxAge, err = s.createToken(&cp, true)

		if err != nil {
			return nil, err
		}

		if err := s.svc.CreateToken(ctx, model.TokenWhitelist{
			Value:     refresh,
			ExpiresAt: claims.ExpiresAt.Time,
			UserID:    model.UserID(claims.UserID),
		}); err != nil {
			return nil, err
		}
	}

	return &pb.UserTokenResponse{
		AccessToken:   access,
		RefreshToken:  refresh,
		AccessMaxAge:  accessMaxAge,
		RefreshMaxAge: refreshMaxAge,
	}, nil
}
