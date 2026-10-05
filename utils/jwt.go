package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/MicahParks/keyfunc/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Claims 访问令牌的声明。
//
// 形状必须与认证中心（call-auth/utils/jwt.go）签发的一致。注意用户 id 在
// **sub** 里 —— JWT 规范要求 sub 是字符串，不是自定义的 user_id 字段
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// UserID 把 sub 转成用户 id
func (c *Claims) UserID() (uint, error) {
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("token 的 sub 不是合法的用户 id: %q", c.Subject)
	}
	return uint(id), nil
}

// IsAdmin 是否管理员。角色来自 token 的 claims，不查库 ——
// 所以改了角色要等 token 过期（最长 15 分钟）才生效
func (c *Claims) IsAdmin() bool {
	return c.Role == "admin"
}

const (
	// jwksRefreshInterval 后台刷新公钥的间隔。认证中心轮换密钥后，
	// 各服务最迟在这个时间之后都能拿到新公钥
	jwksRefreshInterval = time.Hour
	// tokenLeeway 允许的时钟偏差。签发方和验签方的时钟不可能完全一致，
	// 不留余量会出现「刚签出来就说尚未生效」的诡异 401
	tokenLeeway = 60 * time.Second
)

var (
	jwks             *keyfunc.JWKS
	expectedIssuer   string
	expectedAudience string
)

// SetupJWKS 拉取认证中心的公钥集并启动后台刷新。
//
// 本服务**只验签、不签发**：签发权归认证中心，它持私钥，我们拿公钥。
// 这样新增服务不用分发任何密钥，也保证任何一个服务被攻破都伪造不了 token
// （《认证中心设计文档》§3.2）。
//
// cachePath 是公钥的本地缓存：认证中心拉不到时用它兜底启动。没有这个兜底的话，
// 「认证中心没起 + 本服务重启」会直接导致全站 401 —— 连缓存都没有，一个 token 都验不了
func SetupJWKS(jwksURL, issuer, audience, cachePath string) error {
	expectedIssuer = issuer
	expectedAudience = audience

	options := keyfunc.Options{
		Ctx:             context.Background(),
		RefreshInterval: jwksRefreshInterval,
		RefreshErrorHandler: func(err error) {
			// 刷新失败**不致命**：内存里还有上一次的公钥，验签照常。
			// 但若一直失败，认证中心轮换密钥之后就会开始出现验不过的 token
			log.Printf("Warning: 刷新认证中心公钥失败（已缓存的公钥仍然有效）: %v", err)
		},
	}

	var err error
	jwks, err = keyfunc.Get(jwksURL, options)
	if err != nil {
		if fallback := loadCachedJWKS(cachePath); fallback != nil {
			log.Printf("Warning: 拿不到认证中心公钥（%v），暂用本地缓存启动。"+
				"若认证中心在此期间换过密钥，验签会失败", err)
			jwks = fallback
			return nil
		}
		return fmt.Errorf("拉取认证中心公钥失败（%s），且没有可用的本地缓存: %w", jwksURL, err)
	}

	// 拉到了就更新缓存，供下次启动兜底
	if cachePath != "" {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
			log.Printf("Warning: 建公钥缓存目录失败: %v", err)
		} else if err := os.WriteFile(cachePath, jwks.RawJWKS(), 0o600); err != nil {
			log.Printf("Warning: 写公钥缓存失败: %v", err)
		}
	}

	log.Printf("已加载认证中心公钥：%s（%d 把），issuer=%s aud=%s",
		jwksURL, jwks.Len(), issuer, audience)
	return nil
}

// loadCachedJWKS 读本地缓存的公钥集。读不到或内容坏了都返回 nil，由调用方决定怎么办
func loadCachedJWKS(path string) *keyfunc.JWKS {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cached, err := keyfunc.NewJSON(raw)
	if err != nil {
		log.Printf("Warning: 本地公钥缓存已损坏（%s）: %v", path, err)
		return nil
	}
	return cached
}

// ParseToken 解析并校验访问令牌
func ParseToken(tokenString string) (*Claims, error) {
	if jwks == nil {
		return nil, errors.New("公钥未初始化：启动时未调用 SetupJWKS")
	}

	// 显式锁死算法。JWKS 里目前只有 RSA 公钥，但把校验写在明处 ——
	// 万一将来有人往公钥集里加了别的类型的密钥，不至于悄悄放宽验签
	keyLookup := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("意外的签名算法: %v", token.Header["alg"])
		}
		return jwks.Keyfunc(token)
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, keyLookup,
		jwt.WithIssuer(expectedIssuer),
		// aud 必须包含本服务。没有这条，签给别的服务的 token 能拿来调这里
		jwt.WithAudience(expectedAudience),
		// 没有 exp 的 token 一律拒收。光"校验 exp"不够：不要求存在的话，
		// 一个不带 exp 的 token 会直接绕过过期检查
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(tokenLeeway),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
