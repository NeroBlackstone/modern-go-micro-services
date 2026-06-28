package service

import (
	"errors"
	"time"

	"modern-micro-services/internal/user/config"
	"modern-micro-services/internal/user/model"
	"modern-micro-services/internal/user/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Claims JWT 声明
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type UserService interface {
	Register(req *model.RegisterRequest) (*model.LoginResponse, error)
	Login(req *model.LoginRequest) (*model.LoginResponse, error)
	GetProfile(userID uint) (*model.UserResponse, error)
	UpdateProfile(userID uint, req *model.UpdateUserRequest) (*model.UserResponse, error)
	GetUserByID(userID uint) (*model.User, error) // 供 gRPC 调用
}

type userService struct {
	userRepo repository.UserRepository
	jwtCfg   *config.JWTConfig
}

func NewUserService(userRepo repository.UserRepository, jwtCfg *config.JWTConfig) UserService {
	return &userService{
		userRepo: userRepo,
		jwtCfg:   jwtCfg,
	}
}

func (s *userService) GenerateToken(userID uint, email string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.jwtCfg.ExpireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtCfg.Secret))
}

func (s *userService) Register(req *model.RegisterRequest) (*model.LoginResponse, error) {
	existing, _ := s.userRepo.FindByEmail(req.Email)
	if existing != nil {
		return nil, errors.New("email already registered")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	token, err := s.GenerateToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponse{
		Token: token,
		User: model.UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

func (s *userService) Login(req *model.LoginRequest) (*model.LoginResponse, error) {
	user, err := s.userRepo.FindByEmail(req.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid email or password")
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid email or password")
	}

	token, err := s.GenerateToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}

	return &model.LoginResponse{
		Token: token,
		User: model.UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

func (s *userService) GetProfile(userID uint) (*model.UserResponse, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, err
	}

	return &model.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}, nil
}

func (s *userService) UpdateProfile(userID uint, req *model.UpdateUserRequest) (*model.UserResponse, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, err
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return &model.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}, nil
}

func (s *userService) GetUserByID(userID uint) (*model.User, error) {
	return s.userRepo.FindByID(userID)
}
