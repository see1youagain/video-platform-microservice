package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	tests "video-platform-tests"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║   Video Platform Microservice — 集成测试套件               ║")
	fmt.Println("║   Target: http://127.0.0.1:8080                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Print("⏳ 等待 Gateway 就绪 (最多 30s) ... ")
	if !tests.WaitForReady(30 * time.Second) {
		fmt.Println("\n❌ Gateway 未响应！请先启动所有服务。")
		os.Exit(1)
	}
	fmt.Println("OK")

	// 用法：
	// go run ./cmd s23
	// go run ./cmd s4s7
	// go run ./cmd long
	// SAMEHASH_ROUNDS=20 go run ./cmd samehash
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "s23":
			tests.RunStressS2S3()
			return
		case "s4s7":
			tests.RunStressS4S7()
			return
		case "long":
			tests.RunLongSession100Chunks()
			return
		case "samehash":
			rounds := 10
			if v := os.Getenv("SAMEHASH_ROUNDS"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					rounds = n
				}
			}
			tests.RunSameHashMerge50(rounds)
			return
		}
	}

	totalPass, totalFail := 0, 0

	p, f := tests.RunBasicTests()
	totalPass += p
	totalFail += f

	p, f = tests.RunFunctionalTests()
	totalPass += p
	totalFail += f

	p, f = tests.RunConcurrentTests()
	totalPass += p
	totalFail += f

	tests.RunStressTests()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	total := fmt.Sprintf("%d passed   %d failed", totalPass, totalFail)
	padding := 54 - len(total)
	pad := ""
	for i := 0; i < padding; i++ {
		pad += " "
	}
	fmt.Printf("║  %s%s║\n", total, pad)
	fmt.Println("╚══════════════════════════════════════════════════════════╝")

	if totalFail > 0 {
		os.Exit(1)
	}
}
