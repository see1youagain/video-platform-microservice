package validator

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

func ValidateUsername(username string) error {
    if username == "" {
        return fmt.Errorf("用户名不能为空")
    }
    length := utf8.RuneCountInString(username)
    if length < 3 || length > 20 {
        return fmt.Errorf("用户名长度必须在 3-20 个字符之间")
    }
    matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, username)
    if !matched {
        return fmt.Errorf("用户名只能包含字母、数字和下划线")
    }
    return nil
}

func ValidatePassword(password string) error {
    if password == "" {
        return fmt.Errorf("密码不能为空")
    }
    length := len(password)
    if length < 6 || length > 32 {
        return fmt.Errorf("密码长度必须在 6-32 个字符之间")
    }
    return nil
}

func ValidateFileHash(hash string) error {
    if hash == "" {
        return fmt.Errorf("文件哈希不能为空")
    }
    length := len(hash)
    if length != 32 && length != 64 {
        return fmt.Errorf("文件哈希长度不正确")
    }
    matched, _ := regexp.MatchString(`^[a-fA-F0-9]+$`, hash)
    if !matched {
        return fmt.Errorf("文件哈希格式不正确")
    }
    return nil
}
