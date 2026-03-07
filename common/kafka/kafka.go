// Package kafka 提供基于 segmentio/kafka-go 的真实 Kafka 客户端封装。
// 通过 KAFKA_BROKERS 环境变量配置 broker 地址（默认 127.0.0.1:9092）。
package kafka

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Message 轻量消息结构，兼容旧接口。
type Message struct {
	Topic string
	Key   []byte
	Value []byte
	Time  time.Time
}

// ─── Broker 配置 ──────────────────────────────────────────────────────────────

func brokers() []string {
	if addr := os.Getenv("KAFKA_BROKERS"); addr != "" {
		return strings.Split(addr, ",")
	}
	return []string{"127.0.0.1:9092"}
}

// ─── 生产者 ──────────────────────────────────────────────────────────────────

var (
	writerMu sync.Mutex
	writers  = make(map[string]*kafka.Writer)
)

func getWriter(topic string) *kafka.Writer {
	writerMu.Lock()
	defer writerMu.Unlock()
	if w, ok := writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers()...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		Async:        false,
		RequiredAcks: kafka.RequireOne,
		MaxAttempts:  5,
	}
	writers[topic] = w
	return w
}

// InitKafkaProducer 向后兼容（真实 Kafka 不需要显式初始化）。
func InitKafkaProducer(topic string) error {
	_ = getWriter(topic)
	return nil
}

// Publish 发布消息到指定 topic。
func Publish(ctx context.Context, topic, key, value string) error {
	w := getWriter(topic)
	err := w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: []byte(value),
	})
	if err != nil {
		log.Printf("[kafka] Publish to %s failed: %v", topic, err)
	}
	return err
}

// SendMessage 向后兼容接口。
func SendMessage(ctx context.Context, key, value string) error {
	return fmt.Errorf("SendMessage is deprecated, use Publish(ctx, topic, key, value)")
}

func CloseProducer() error {
	writerMu.Lock()
	defer writerMu.Unlock()
	for _, w := range writers {
		_ = w.Close()
	}
	writers = make(map[string]*kafka.Writer)
	return nil
}

// ─── 消费者 ──────────────────────────────────────────────────────────────────

// Reader 封装 kafka.Reader。
type Reader struct {
	r *kafka.Reader
}

// NewReader 创建 Kafka 消费者，新 consumer group 从最早偏移量开始，已有 group 从上次提交的位置继续。
func NewReader(topic, groupID string) *Reader {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers(),
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        500 * time.Millisecond,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
	return &Reader{r: r}
}

// ReadMessage 阻塞等待下一条消息。
func (r *Reader) ReadMessage(ctx context.Context) (Message, error) {
	msg, err := r.r.ReadMessage(ctx)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Topic: msg.Topic,
		Key:   msg.Key,
		Value: msg.Value,
		Time:  msg.Time,
	}, nil
}

func (r *Reader) Close() error { return r.r.Close() }

// ─── 向后兼容的全局 Reader ───────────────────────────────────────────────────

var (
	globalReaderMu sync.Mutex
	globalReader   *Reader
)

func InitKafkaConsumer(topic, groupID string) error {
	globalReaderMu.Lock()
	defer globalReaderMu.Unlock()
	globalReader = NewReader(topic, groupID)
	return nil
}

func ReadMessage(ctx context.Context) (Message, error) {
	globalReaderMu.Lock()
	r := globalReader
	globalReaderMu.Unlock()
	if r == nil {
		return Message{}, fmt.Errorf("kafka reader not initialized")
	}
	return r.ReadMessage(ctx)
}

func CloseConsumer() error { return nil }
