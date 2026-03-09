package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"call-go/utils"
	"errors"
)

type AuthService struct{}

// Register 注册用户
func (s *AuthService) Register(req *dto.RegisterRequest) (*dto.LoginResponse, error) {
	// 检查用户名是否已存在
	var existingUser models.User
	err := config.DB.Where("username = ?", req.Username).First(&existingUser).Error
	if err == nil {
		return nil, errors.New("用户名已存在")
	}

	// 创建新用户
	user := &models.User{
		Username: req.Username,
		Nickname: req.Nickname,
		Status:   "active",
	}

	if err := user.SetPassword(req.Password); err != nil {
		return nil, errors.New("密码加密失败")
	}

	if err := config.DB.Create(user).Error; err != nil {
		return nil, errors.New("注册失败")
	}

	// 生成 token
	token, err := utils.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, errors.New("生成token失败")
	}

	return &dto.LoginResponse{
		Token: token,
		User: dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		},
	}, nil
}

// Login 登录
func (s *AuthService) Login(req *dto.LoginRequest) (*dto.LoginResponse, error) {
	// 查找用户
	var user models.User
	err := config.DB.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 检查用户状态
	if user.Status != "active" {
		return nil, errors.New("账号已被禁用")
	}

	// 验证密码
	if !user.CheckPassword(req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	// 生成 token
	token, err := utils.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, errors.New("生成token失败")
	}

	return &dto.LoginResponse{
		Token: token,
		User: dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		},
	}, nil
}

// GetUserInfo 获取用户信息
func (s *AuthService) GetUserInfo(userID uint) (*dto.UserInfo, error) {
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	return &dto.UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
	}, nil
}
