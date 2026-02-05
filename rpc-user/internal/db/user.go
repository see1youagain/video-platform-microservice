package db

import (
	"time"

	"gorm.io/gorm"
)

// 初始化数据库连接 (标准 Gorm 连接代码)
var DB *gorm.DB

type User struct {
	ID        uint      `gorm:"primaryKey"`
	Username  string    `gorm:"uniqueIndex;type:varchar(100);not null"`
	Password  string    `gorm:"not null"` // 存储 bcrypt 哈希后的字符串，不是明文
	CreatedAt time.Time
}

// CreateUser 创建用户
func CreateUser(username, password string) (uint, error) {
    user := User{
        Username: username,
        Password: password,
    }
    
    // 调用 GORM 创建用户
    if err := DB.Create(&user).Error; err != nil {
        return 0, err
    }
    
    return user.ID, nil
}

// GetUserByUsername 根据用户名查询
func GetUserByUsername(username string) (*User, error) {
    var user User
    
    // 使用 GORM 的 Where 查询
    if err := DB.Where("username = ?", username).First(&user).Error; err != nil {
        return nil, err
    }
    
    return &user, nil
}