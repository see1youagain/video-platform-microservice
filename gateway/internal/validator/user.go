package validator

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

// ValidateUsername 验证用户名
// 规则：3-20 个字符，只允许字母、数字、下划线
func ValidateUsername(username string) error {
	length := utf8.RuneCountInString(username)
	if length < 3 {
		return errors.New("用户名长度不能少于 3 个字符")
	}
	if length > 20 {
		return errors.New("用户名长度不能超过 20 个字符")
	}

	// 只允许字母、数字、下划线
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	if !matched {
		return errors.New("用户名只能包含字母、数字和下划线")
	}

	return nil
}

// ValidatePassword 验证密码
// 规则：至少 6 个字符
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < 6 {
		return errors.New("密码长度不能少于 6 个字符")
	}
	if length > 32 {
		return errors.New("密码长度不能超过 32 个字符")
	}

	return nil
}
