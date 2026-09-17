package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 用户角色
const (
	UserRoleUser  = "user"  // 普通用户
	UserRoleAdmin = "admin" // 系统管理：目前唯一的特权是审批标签入库
)

// User 用户表
type User struct {
	ID       uint   `json:"id" gorm:"primaryKey"`
	Username string `json:"username" gorm:"size:100;uniqueIndex;not null;comment:用户名"`
	Nickname string `json:"nickname" gorm:"size:100;comment:昵称"`
	Avatar   string `json:"avatar" gorm:"size:500;comment:头像URL"`
	Password string `json:"-" gorm:"size:255;not null;comment:密码"`
	Status   string `json:"status" gorm:"size:20;default:'active';comment:状态: active-正常, inactive-禁用"`
	// Role 刻意不做任何自动提升：管理员一律手工指定
	// （UPDATE users SET role='admin' WHERE username='xxx'），
	// 避免"第一个注册的人自动变管理员"这类隐式提权
	Role      string         `json:"role" gorm:"size:20;not null;default:'user';comment:角色: user-普通用户, admin-系统管理"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// IsAdmin 是否系统管理
func (u *User) IsAdmin() bool {
	return u.Role == UserRoleAdmin
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// SetPassword 设置密码（加密）
func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}
