package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	ErrorRate       float64
	MinSamples      int64
	SuccessRate     float64
	Timeout         time.Duration
	HalfOpenTimeout time.Duration
}

func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ErrorRate:       0.5,
		MinSamples:      100,
		SuccessRate:     0.6,
		Timeout:         30 * time.Second,
		HalfOpenTimeout: 10 * time.Second,
	}
}

// NewKitexCircuitBreaker 预留给业务层注入Kitex熔断option
func NewKitexCircuitBreaker(_ CircuitBreakerConfig) interface{} {
	return nil
}

type CircuitBreaker struct {
	mu              sync.RWMutex
	state           State
	config          CircuitBreakerConfig
	successCount    int64
	failureCount    int64
	lastFailureTime time.Time
	halfOpenTime    time.Time
}

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{state: StateClosed, config: config}
}

func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	_ = ctx
	if !cb.AllowRequest() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}
	cb.RecordSuccess()
	return nil
}

func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.config.HalfOpenTimeout {
			cb.state = StateHalfOpen
			cb.halfOpenTime = time.Now()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successCount++
	if cb.state == StateHalfOpen {
		totalCount := cb.successCount + cb.failureCount
		if totalCount >= cb.config.MinSamples {
			successRate := float64(cb.successCount) / float64(totalCount)
			if successRate >= cb.config.SuccessRate {
				cb.state = StateClosed
				cb.reset()
			}
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	totalCount := cb.successCount + cb.failureCount
	if totalCount >= cb.config.MinSamples {
		errorRate := float64(cb.failureCount) / float64(totalCount)
		if errorRate >= cb.config.ErrorRate {
			cb.state = StateOpen
		}
	}

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.reset()
	}
}

func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	totalCount := cb.successCount + cb.failureCount
	errorRate := 0.0
	if totalCount > 0 {
		errorRate = float64(cb.failureCount) / float64(totalCount)
	}

	return map[string]interface{}{
		"state":         cb.state.String(),
		"success_count": cb.successCount,
		"failure_count": cb.failureCount,
		"total_count":   totalCount,
		"error_rate":    errorRate,
	}
}

func (cb *CircuitBreaker) reset() {
	cb.successCount = 0
	cb.failureCount = 0
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.reset()
}
