package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword 密码加密
func HashPassword(password string) (string, error) {
bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
return string(bytes), err
}

// CheckPasswordHash 密码比对
func CheckPasswordHash(password, hash string) bool {
err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
return err == nil
}
