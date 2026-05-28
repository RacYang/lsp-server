package handler

import "racoo.cn/lsp/internal/contract"

// ResumeResult 是断线重连恢复结果的类型别名，定义在 contract 包，避免 gateway → handler 反向依赖。
type ResumeResult = contract.ResumeResult

// ResumeError 是重连链路业务错误的类型别名，定义在 contract 包。
type ResumeError = contract.ResumeError
