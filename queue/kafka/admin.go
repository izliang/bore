package kafka

import (
	"context"
	"fmt"
	"net"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
)

// ensureTopic 确保指定 topic 存在。如果 topic 不存在，自动创建。
//
// 该函数是幂等的：topic 已存在时直接返回 nil。
// 自动创建行为默认启用，通过 Options.AutoCreateTopics 可显式禁用。
// 检查 topic 是否存在通过 Metadata API 实现。
func (q *kafkaQueue) ensureTopic(ctx context.Context, topicName string, opts *queue.DeclareOptions) error {
	// 检查 topic 是否已存在
	metadata, err := q.admin.Metadata(ctx, &kafka.MetadataRequest{
		Topics: []string{topicName},
	})
	if err != nil {
		return fmt.Errorf("获取 topic 元数据失败 %q: %w", topicName, err)
	}

	// 如果 topic 存在且无错误，直接返回
	if len(metadata.Topics) > 0 && metadata.Topics[0].Error == nil {
		return nil
	}

	// 使用默认配置创建 topic
	partitions := q.opts.DefaultPartitions
	replicationFactor := q.opts.DefaultReplicationFactor
	retention := q.opts.DefaultRetention

	// 根据 DeclareOptions 调整（如果有）
	if opts != nil {
		if opts.MaxLength > 0 {
			// 根据最大长度估算分区数：每 1000 条消息一个分区
			estimated := int(opts.MaxLength / 1000)
			if estimated > partitions {
				partitions = estimated
			}
		}
	}

	resp, err := q.admin.CreateTopics(ctx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{{
			Topic:             topicName,
			NumPartitions:     partitions,
			ReplicationFactor: replicationFactor,
			ConfigEntries: []kafka.ConfigEntry{
				{ConfigName: "retention.ms", ConfigValue: fmt.Sprintf("%d", retention.Milliseconds())},
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("创建 topic %q 失败: %w", topicName, err)
	}

	if len(resp.Errors) > 0 {
		for _, e := range resp.Errors {
			if e != nil {
				return fmt.Errorf("创建 topic %q 失败: %v", topicName, e)
			}
		}
	}

	return nil
}

// dialKafka 建立与 Kafka broker 的初始连接，用于验证连接可用性。
func (q *kafkaQueue) dialKafka(ctx context.Context) error {
	dialer := &kafka.Dialer{
		Timeout:   q.opts.DialTimeout,
		DualStack: true,
	}
	if q.opts.TLS != nil {
		dialer.TLS = q.opts.TLS
	}

	var lastErr error
	for _, broker := range q.opts.Brokers {
		conn, err := dialer.DialContext(ctx, "tcp", broker)
		if err != nil {
			lastErr = err
			continue
		}
		conn.Close()
		return nil
	}
	return fmt.Errorf("无法连接任何 Kafka broker: %w", lastErr)
}

// resolverFunc 返回用于 kafka.Dialer 的地址解析函数。
func (q *kafkaQueue) resolverFunc() func(network, addr string) (net.Addr, error) {
	return func(network, addr string) (net.Addr, error) {
		return net.ResolveTCPAddr(network, addr)
	}
}
