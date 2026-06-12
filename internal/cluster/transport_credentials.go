package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// 集群内 gRPC 传输凭据的唯一构造点。凭据决策归属：config 持值、本文件构造、
// 进程装配注入；任何调用点不得自行决定凭据形态。
//
// 语义约定：
//   - 三项文件（证书、私钥、CA）齐备 → 双向 mTLS；
//   - 三项全空 → 显式明文（Alpha 阶段可信网络默认，装配处必须输出警告日志）；
//   - 半配置 → 返回错误拒绝启动，禁止静默降级为明文。

// NewServerTransportCredentials 构造集群 gRPC 服务端凭据；mTLS 模式下强制校验客户端证书。
func NewServerTransportCredentials(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	switch {
	case certFile == "" && keyFile == "" && caFile == "":
		return insecure.NewCredentials(), nil
	case certFile == "" || keyFile == "" || caFile == "":
		return nil, fmt.Errorf("集群 TLS 半配置：cert/key/ca 三项必须同时提供或同时为空")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载集群服务端证书: %w", err)
	}
	pool, err := loadCertPool(caFile)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}), nil
}

// NewClientTransportCredentials 构造集群 gRPC 客户端凭据；serverName 仅在证书 SAN
// 与拨号地址不一致时需要（如经由负载均衡 IP 访问）。
func NewClientTransportCredentials(certFile, keyFile, caFile, serverName string) (credentials.TransportCredentials, error) {
	tlsCfg, err := NewClientTLSConfig(certFile, keyFile, caFile, serverName)
	if err != nil {
		return nil, err
	}
	if tlsCfg == nil {
		return insecure.NewCredentials(), nil
	}
	return credentials.NewTLS(tlsCfg), nil
}

// NewClientTLSConfig 构造集群客户端 *tls.Config，供 gRPC 与 etcd 等出站连接复用；
// 三项全空返回 nil 表示明文，半配置返回错误。
func NewClientTLSConfig(certFile, keyFile, caFile, serverName string) (*tls.Config, error) {
	switch {
	case certFile == "" && keyFile == "" && caFile == "":
		return nil, nil
	case certFile == "" || keyFile == "" || caFile == "":
		return nil, fmt.Errorf("集群 TLS 半配置：cert/key/ca 三项必须同时提供或同时为空")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载集群客户端证书: %w", err)
	}
	pool, err := loadCertPool(caFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caFile) //nolint:gosec // G304：路径来自部署配置（cluster.tls/etcd.tls），非外部输入
	if err != nil {
		return nil, fmt.Errorf("读取集群 CA 证书: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("解析集群 CA 证书失败: %s", caFile)
	}
	return pool, nil
}
