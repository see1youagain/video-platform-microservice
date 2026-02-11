package utils

import (
	"errors"
	"log"
	"os"

	"github.com/golang-jwt/jwt/v5"
)

var JwtSecret []byte

// InitJWT 初始化 JWT 密钥
func InitJWT() error {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return errors.New("JWT_SECRET 环境变量未设置")
	}
	if len(secret) < 32 {
		log.Println("警告: JWT_SECRET 长度过短，建议至少 32 个字符")
	}
	JwtSecret = []byte(secret)
	log.Println("Gateway JWT 密钥初始化成功")
	return nil
}

// ParseToken 解析 Token
func ParseToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return JwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
