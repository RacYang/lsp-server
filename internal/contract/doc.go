// Package contract 定义 handler、gateway、adapter 之间的共享契约类型。
//
// 职责：声明跨层接口（RoomGateway）与共享错误变量，解耦 handler 与实现层。
// 禁止在本包内存放任何实现代码；只允许接口、类型定义与哨兵错误变量。
package contract
