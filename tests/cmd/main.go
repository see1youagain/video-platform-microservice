package main

import (
	"fmt"
	"os"
	"time"

	tests "video-platform-tests"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║   Video Platform Microservice — 集成测试套件          ║")
	fmt.Println("║   Target: http://127.0.0.1:8080                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Print("⏳ 等待 Gateway 就绪 (最多 30s) ... ")
	if !tests.WaitForReady(30 * time.Second) {
		fmt.Println("\n❌ Gateway 未响应！请先启动所有服务。")
		fmt.Println("   运行: ./scripts/start_all.sh")
		os.Exit(1)
	}
	fmt.Println("OK")

	totalPass, totalFail := 0, 0

	p, f := tests.RunBasicTests()
	totalPass += p
	totalFail += f

	p, f = tests.RunFunctionalTests()
	totalPass += p
	totalFail += f

	// Phase 3: Stress（先注释，优先验证功能测试）
	// tests.RunStressTests()

	fmt.Println("\n╔══════════════════════════════════════════════════════╗")
	fmt.Printf("║  总计: %d passed   %d failed", totalPass, totalFail)
	padding := 38 - len(fmt.Sprintf("%d passed   %d failed", totalPass, totalFail))
	for i := 0; i < padding; i++ {
		fmt.Print(" ")
	}
	fmt.Println("║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")

	if totalFail > 0 {
		os.Exit(1)
	}
}
