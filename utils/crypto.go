package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"strings"
)

// aes256KeySize AES-256 的密钥字节数
const aes256KeySize = 32

// secretHintMinLen 短于这个长度就不给掩码。
// 掩码只露末 4 位，若 Key 本身只有 6 个字符，露 4 位等于把大部分内容送出去
const secretHintMinLen = 8

// ErrEncryptionUnavailable 主密钥缺失导致无法加密。
//
// 单独做一个哨兵错误，是为了让 controller 能区分"服务端没配好"（该报 500）
// 与"用户填错了"（该报 400）—— 把服务端配置问题报成 400，用户会去反复检查
// 自己填的 Key，而问题根本不在那儿
var ErrEncryptionUnavailable = errors.New("服务端未配置加密密钥，暂时无法保存模型设置")

// preferenceEncryptionKey 用户 API Key 的加密主密钥。
//
// 与 jwtSecret 同一个模式：包级变量 + main 启动时注入。
// 为 nil 表示未配置，此时加密写路径必须明确报错，见 EncryptSecret
var preferenceEncryptionKey []byte

// SetPreferenceEncryptionKey 设置用户 API Key 的加密主密钥。
//
// 期望 32 字节：既接受 `openssl rand -base64 32` 的输出（先按 base64 解），
// 也接受长度恰好 32 字节的原始串。
//
// 刻意**不做**两件事：
//  1. 密钥不合法时不兜底生成一把临时密钥 —— 那会让每次重启之后已存的密文
//     全部解不开，用户的 Key 静默变成垃圾
//  2. 不拿 JWT_SECRET 派生兜底 —— 那会让轮换 JWT 密钥顺手毁掉全部模型配置
//
// 宁可让写路径明确报错，也不能把「读不出来」变成「悄悄换了一把钥匙」。
func SetPreferenceEncryptionKey(raw string) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		preferenceEncryptionKey = nil
		log.Println("Warning: PREF_ENCRYPTION_KEY 未配置，「模型设置」只能读不能写")
		return
	}

	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == aes256KeySize {
		preferenceEncryptionKey = decoded
		return
	}
	if len(raw) == aes256KeySize {
		preferenceEncryptionKey = []byte(raw)
		return
	}

	preferenceEncryptionKey = nil
	log.Printf("Warning: PREF_ENCRYPTION_KEY 长度不合法（需 32 字节，或 base64 编码的 32 字节），「模型设置」只能读不能写")
}

// EncryptionReady 主密钥是否可用
func EncryptionReady() bool {
	return len(preferenceEncryptionKey) == aes256KeySize
}

// EncryptSecret 用 AES-256-GCM 加密一段密钥。
//
// 输出 base64(nonce || ciphertext+tag)，nonce 每次随机 —— 同一把 Key 存两次
// 得到的密文不同，从密文上看不出两个用户是否用了同一把 Key。
func EncryptSecret(plain string) (string, error) {
	if !EncryptionReady() {
		return "", ErrEncryptionUnavailable
	}

	block, err := aes.NewCipher(preferenceEncryptionKey)
	if err != nil {
		return "", errors.New("加密失败: " + err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("加密失败: " + err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.New("生成随机数失败: " + err.Error())
	}

	// Seal 的第一个参数是 dst：把 nonce 本身当 dst，密文就紧跟在 nonce 之后
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret 解密 EncryptSecret 的输出。
//
// 主密钥缺失或换过时都会失败 —— 调用方要把这两种情况翻译成
// 「请重新填写 API Key」的人话，而不是把这里的错误原文抛给用户
func DecryptSecret(encoded string) (string, error) {
	if !EncryptionReady() {
		return "", errors.New("服务端未配置加密密钥")
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("密钥密文格式非法")
	}

	block, err := aes.NewCipher(preferenceEncryptionKey)
	if err != nil {
		return "", errors.New("解密失败: " + err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("解密失败: " + err.Error())
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("密钥密文长度不足")
	}

	// GCM 自带认证：密文被改过一个字节这里就会失败
	opened, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("密钥解密失败（主密钥可能已更换）")
	}
	return string(opened), nil
}

// SecretHint 取末 4 位，供设置页显示掩码。
//
// 明文存这一小段是刻意的：主密钥缺失或轮换后密文解不开，
// 但靠它仍能告诉用户「你配过一把 Key」，报错与界面才不会互相矛盾。
func SecretHint(plain string) string {
	runes := []rune(plain)
	if len(runes) < secretHintMinLen {
		return ""
	}
	return string(runes[len(runes)-4:])
}
