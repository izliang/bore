package kafka

import "strings"

// resolveTopic 将队列名解析为 Kafka topic 名。
// 格式：{namespace}.{queueName}
//
// 如果 namespace 为空，默认使用 "default"。
func resolveTopic(namespace, queueName string) string {
	if namespace == "" {
		namespace = "default"
	}
	return namespace + "." + queueName
}

// resolveConsumerGroup 生成 Consumer Group ID。
// 格式：{prefix}:{namespace}:{queueName}
//
// 如果 prefix 为空，则格式为 {namespace}:{queueName}。
func resolveConsumerGroup(prefix, namespace, queueName string) string {
	if namespace == "" {
		namespace = "default"
	}
	if prefix != "" {
		return prefix + ":" + namespace + ":" + queueName
	}
	return namespace + ":" + queueName
}

// resolveRetryTopic 解析重试 topic 名。
// 格式：{namespace}.{queueName}{suffix}
func resolveRetryTopic(namespace, queueName, suffix string) string {
	return resolveTopic(namespace, queueName) + suffix
}

// resolveDLQTopic 解析死信 topic 名。
// 格式：{namespace}.{queueName}{suffix}
func resolveDLQTopic(namespace, queueName, suffix string) string {
	return resolveTopic(namespace, queueName) + suffix
}

// parseTopicName 从 topic 名中提取命名空间前缀并返回队列名。
// 如果 topic 不以 "{namespace}." 开头，返回原始 topic 名。
//
// 例如 "production.orders.retry" → "orders.retry"（当 namespace="production"）
func parseTopicName(namespace, topic string) string {
	if namespace == "" {
		namespace = "default"
	}
	prefix := namespace + "."
	if strings.HasPrefix(topic, prefix) {
		return topic[len(prefix):]
	}
	return topic
}

// isTopicInNamespace 检查 topic 是否属于指定命名空间。
func isTopicInNamespace(namespace, topic string) bool {
	if namespace == "" {
		namespace = "default"
	}
	return strings.HasPrefix(topic, namespace+".")
}
