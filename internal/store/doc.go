// Package store 提供 Redis 与 PostgreSQL 共享的存储弹性辅助函数。
//
// 职责：WithOperationTimeout、IsRetryable、Retry 等基础重试工具。
// 子包 redis/ 与 postgres/ 实现具体存储访问；本包不承载任何存储连接或业务对象。
package store
