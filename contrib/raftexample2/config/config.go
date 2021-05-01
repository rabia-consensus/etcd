package config

import "time"

var Conf Config

type Config struct {
	NClients   int
	KeyLen     int
	ValLen     int
	LenChannel int
	TcpBufSize int
	IoBufSize  int

	ClosedLoop        bool
	ClientBatchSize   int
	ClientThinkTime   int
	ClientTimeout     time.Duration
	ClientLogInterval time.Duration
	NClientRequests   int

	ProxyBatchSize    int
	ProxyBatchTimeout time.Duration
}

func (c *Config) LoadConfigs() {
	c.NClients = 1
	c.KeyLen = 8
	c.ValLen = 8
	c.LenChannel = 500000
	c.TcpBufSize = 7000000
	c.IoBufSize = 4096 * 4000

	c.ClosedLoop = true
	c.ClientBatchSize = 1
	c.ClientThinkTime = 1 // in ms
	c.ClientTimeout = 20 * time.Second
	c.ClientLogInterval = 5 * time.Second
	c.NClientRequests = 10000000

	c.ProxyBatchSize = 1
	c.ProxyBatchTimeout = 5 * time.Millisecond
}
