package tests

import (
	"fmt"
	"net/http"
)

// RunBasicTests tests connectivity, auth, user registration/login
func RunBasicTests() (passed, failed int) {
	s := NewSuite("BASIC")
	fmt.Println("\n\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550")
	fmt.Println("  PHASE 1 \u2014 \u57fa\u7840\u8fde\u901a\u6027\u6d4b\u8bd5")
	fmt.Println("\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550")

	c := NewClient()

	// 1. Ping
	fmt.Println("\n[1] Ping")
	resp, body, err := c.GET("/ping", nil)
	s.Check("GET /ping \u2192 200", err == nil && sc(resp) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", err, sc(resp), body))

	// 2. Register
	fmt.Println("\n[2] \u7528\u6237\u6ce8\u518c")
	username := fmt.Sprintf("tu%d", nowMs()%10000000)
	resp, body, err = c.POST("/api/register", map[string]string{
		"username": username,
		"password": "Test123456",
	})
	s.Check("POST /api/register \u65b0\u7528\u6237 \u2192 200", err == nil && sc(resp) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", err, sc(resp), body))

	resp2, body2, err2 := c.POST("/api/register", map[string]string{"username": username, "password": "Test123456"})
	s.Check("POST /api/register \u91cd\u590d\u7528\u6237\u540d \u2192 4xx", err2 == nil && sc(resp2) >= 400,
		fmt.Sprintf("status=%v body=%v", sc(resp2), body2))

	// username too short
	resp3, _, _ := c.POST("/api/register", map[string]string{"username": "ab", "password": "Test123456"})
	s.Check("POST /api/register \u7528\u6237\u540d\u592a\u77ed \u2192 400", sc(resp3) == 400, "")

	// weak password (< 6 chars)
	resp4, _, _ := c.POST("/api/register", map[string]string{"username": username + "2", "password": "123"})
	s.Check("POST /api/register \u5f31\u5bc6\u7801 \u2192 400", sc(resp4) == 400, "")

	// 3. Login
	fmt.Println("\n[3] \u7528\u6237\u767b\u5f55")
	resp, body, err = c.POST("/api/login", map[string]string{
		"username": username,
		"password": "Test123456",
	})
	s.Check("POST /api/login \u6b63\u786e\u5bc6\u7801 \u2192 200", err == nil && sc(resp) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", err, sc(resp), body))

	token := ""
	if body != nil {
		if t, ok := body["token"].(string); ok {
			token = t
		}
	}
	s.Check("\u767b\u5f55\u54cd\u5e94\u5305\u542b token", token != "", fmt.Sprintf("body=%v", body))

	// wrong password
	resp5, _, _ := c.POST("/api/login", map[string]string{"username": username, "password": "WrongPwd9"})
	s.Check("POST /api/login \u9519\u8bef\u5bc6\u7801 \u2192 401", sc(resp5) == 401, "")

	// non-existent user
	resp6, _, _ := c.POST("/api/login", map[string]string{"username": "no_such_user_xyz", "password": "Test123456"})
	s.Check("POST /api/login \u4e0d\u5b58\u5728\u7528\u6237 \u2192 4xx", sc(resp6) >= 400, "")

	// 4. Auth protection
	fmt.Println("\n[4] \u9274\u6743\u4fdd\u62a4\u9a8c\u8bc1")
	resp7, _, _ := c.GET("/api/profile", nil)
	s.Check("GET /api/profile \u65e0 token \u2192 401", sc(resp7) == 401, "")

	// wrong format token
	c2 := NewClient()
	c2.Token = "not.a.jwt"
	resp8, _, _ := c2.GET("/api/profile", nil)
	s.Check("GET /api/profile \u65e0\u6548 token \u2192 401", sc(resp8) == 401, "")

	// 5. Profile
	if token != "" {
		c.Token = token
		resp9, body9, err9 := c.GET("/api/profile", nil)
		s.Check("GET /api/profile \u6709\u6548 token \u2192 200", err9 == nil && sc(resp9) == 200,
			fmt.Sprintf("err=%v status=%v body=%v", err9, sc(resp9), body9))
	} else {
		s.Check("GET /api/profile (SKIP: login \u672a\u8fd4\u56de token)", false, "")
	}

	return s.Summary()
}

// sc safely returns http.Response.StatusCode
func sc(r *http.Response) int {
	if r == nil {
		return 0
	}
	return r.StatusCode
}
