package tcp

import (
	"bufio"
	"fmt"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/config"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/msg"
	"net"
	"sync"
	"time"
)

/*
	Generates a reader and a writer from a connection.

	Note: I suspect that if we call this function twice, the newly generated reader and writer will replace the
	previously allocated reader and writer. Be aware of any side-effects.
*/
func GetReaderWriter(conn *net.Conn) (*bufio.Reader, *bufio.Writer) {
	var err error
	err = (*conn).(*net.TCPConn).SetWriteBuffer(Conf.TcpBufSize)
	if err != nil {
		panic("should not happen")
	}
	err = (*conn).(*net.TCPConn).SetReadBuffer(Conf.TcpBufSize)
	if err != nil {
		panic("should not happen")
	}
	err = (*conn).(*net.TCPConn).SetKeepAlive(true)
	if err != nil {
		panic("should not happen")
	}
	err = (*conn).(*net.TCPConn).SetKeepAlivePeriod(20 * time.Second)
	if err != nil {
		panic("should not happen")
	}
	reader := bufio.NewReaderSize(*conn, Conf.IoBufSize)
	writer := bufio.NewWriterSize(*conn, Conf.IoBufSize)
	return reader, writer
}

/*
	Client TCP, each client connects to a single proxy
*/
type ClientTCP struct {
	Id   uint32          // client id
	Wg   *sync.WaitGroup // waits SendHandler and RecvHandler
	Done chan struct{}   // waits SendHandler and RecvHandler

	ProxyAddr string       // the address of a proxy that the client intends to connect to
	RecvChan  chan Command // RecvHandler receives Command objects from Reader and then sends to this channel
	SendChan  chan Command // SendHandler receives Command objects from this channel and sends to Writer

	Conn   *net.Conn     // the connection to a proxy
	Reader *bufio.Reader // the reader that binds to the Conn object
	Writer *bufio.Writer // the writer that binds to the Conn object
}

func ClientTcpInit(Id uint32, ProxyIp string) *ClientTCP {
	c := &ClientTCP{
		Id:   Id,
		Wg:   &sync.WaitGroup{},
		Done: make(chan struct{}),

		ProxyAddr: ProxyIp,
		RecvChan:  make(chan Command, Conf.LenChannel),
		SendChan:  make(chan Command, Conf.LenChannel),
	}
	/*
		Conns, Reader, and Writer fields are not initialized at this point.
	*/
	return c
}

func (c *ClientTCP) connect() {
	for {
		conn, err := net.Dial("tcp", c.ProxyAddr)
		if err == nil {
			c.Conn = &conn
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.Reader, c.Writer = GetReaderWriter(c.Conn)

	cmd := &Command{CliId: c.Id}
	err := cmd.MarshalWriteFlush(c.Writer)
	if err != nil {
		panic(err)
	}
	c.Wg.Add(2)
	go c.RecvHandler()
	go c.SendHandler()
}

func (c *ClientTCP) Connect() {
	c.connect()
	c.PrintStatus()
}

/*
	For each message received, send the message to RecvChan.
	RecvHandler exits when the channel is closed.
*/
func (c *ClientTCP) RecvHandler() {
	defer c.Wg.Done()
	readBuf := make([]byte, 4096*100)
	for {
		var cmd Command
		err := cmd.ReadUnmarshal(c.Reader, readBuf)
		if err != nil { // TCP connection closed
			return
		}
		c.RecvChan <- cmd
	}
}

/*
	For each message in SendChan, marshal the message and flush its bytes to the TCP connection through Writer before
	it exits (when c.Done channel is closed, SendHandler exits).
	Note:

	1. it is likely that c.Done closes before SendHandler sends every message ever passed in SendChan. In that case,
	some messages are remained in the channel but not sent.

	2. if the receiver has closed its connection, it is likely that no error msg is produced here at the sender; if the
	sender has closed its connection, error indeed happens.
*/
func (c *ClientTCP) SendHandler() {
	defer c.Wg.Done()
	for {
		select {
		case <-c.Done:
			return
		case req := <-c.SendChan:
			err := req.MarshalWriteFlush(c.Writer)
			if err != nil {
				panic(err)
			}
		}
	}
}

/*
	Prints the connection status.
*/
func (c *ClientTCP) PrintStatus() {
	fmt.Printf("ClientTcp, SvrId=%d, ProxyAddr=%s, Conns.local=%s, Conns.remote=%s\n",
		c.Id, c.ProxyAddr, (*c.Conn).LocalAddr(), (*c.Conn).RemoteAddr())
}

/*
	Closes the connection and waits SendHandler and RecvHandler to exit.
*/
func (c *ClientTCP) Close() {
	_ = (*c.Conn).Close()
	close(c.Done)
	c.Wg.Wait()
}

/*
	Proxy TCP, each proxy connects to one or more clients
*/
type ProxyTCP struct {
	Id   uint32          // proxy id
	Wg   *sync.WaitGroup // waits SendHandlers and RecvHandlers
	Done chan struct{}   // waits SendHandlers and RecvHandlers

	ProxyAddr string
	RecvChan  chan Command   // from clients to the proxy
	SendChan  []chan Command // from the proxy to clients

	Listener net.Listener
	Conns    []*net.Conn
	Readers  []*bufio.Reader
	Writers  []*bufio.Writer
}

// Allocates the ProxyTCP object without accepting connections from its clients
func ProxyTcpInit(Id uint32, ProxyIp string, ToProxy chan Command) *ProxyTCP {
	listener, err := net.Listen("tcp", ProxyIp)
	if err != nil {
		panic(err)
	}

	p := &ProxyTCP{
		Id:   Id,
		Wg:   &sync.WaitGroup{},
		Done: make(chan struct{}),

		ProxyAddr: ProxyIp,
		RecvChan:  ToProxy,
		SendChan:  make([]chan Command, Conf.NClients), // at most NClients clients can connect to this proxy

		Listener: listener,
		Conns:    make([]*net.Conn, Conf.NClients),
		Readers:  make([]*bufio.Reader, Conf.NClients),
		Writers:  make([]*bufio.Writer, Conf.NClients),
	}
	/*
		Note: SendChan, Conns, Readers, and Writers entries are not initialized at this points.
		Why arrays are of length NClients but not Clients[id]?
		Because the proxy needs to perform quick lookups of clients, see "if p.TCP.Conns[cid] != nil" on proxy.go
	*/
	return p
}

// Waits all its clients to connect
func (p *ProxyTCP) connect() {
	// Conf.NClients is an upper bound, but in common cases,
	// a proxy is connected to (Conf.NClients / Conf.NServers) clients
	for i := 0; i < Conf.NClients; i++ {
		conn, err := p.Listener.Accept()
		if err != nil {
			//fmt.Printf("ProxyTCP%d: connection accept thread exits\n", Conf.SvrId)
			return
		}

		reader, writer := GetReaderWriter(&conn)
		var req Command
		readBuf := make([]byte, 20)
		err = req.ReadUnmarshal(reader, readBuf)
		if err != nil {
			panic(err)
		}

		CliId := req.CliId
		p.Conns[CliId] = &conn
		p.SendChan[CliId] = make(chan Command, Conf.LenChannel)
		p.Writers[CliId] = writer
		p.Readers[CliId] = reader
		p.Wg.Add(2)
		go p.SendHandler(int(CliId))
		go p.RecvHandler(int(CliId))
	}
}

/*
	Proxy's Connect function is asynchronous - it exits early before connection to all clients (but it has a background
	routine that waits connections). The primary purpose of this function is exiting correctly. If we want the proxy to
	exit before connecting to all clients (due to some error or misconfiguration may happened), this function needs to
	be non-blocking ("Listener.Accept()" is blocking).
*/
func (p *ProxyTCP) Connect() {
	go func() { p.connect() }()
}

func (p *ProxyTCP) RecvHandler(from int) {
	defer p.Wg.Done()
	readBuf := make([]byte, 4096*100)
	for {
		var c Command
		err := c.ReadUnmarshal(p.Readers[from], readBuf)
		if err != nil {
			// maybe: TCP connection is closed or receives an ill-formed message
			return
		}
		p.RecvChan <- c
	}
}

func (p *ProxyTCP) SendHandler(to int) {
	defer p.Wg.Done()
	for {
		select {
		case <-p.Done:
			return
		case c := <-p.SendChan[to]:
			err := c.MarshalWriteFlush(p.Writers[to])
			if err != nil {
				return
			}
		}
	}
}

func (p *ProxyTCP) PrintStatus() {
	fmt.Printf("proxyTcp, SvrId=%d, ProxyAddr=%s\n", p.Id, p.ProxyAddr)
	for i := 0; i < Conf.NClients; i++ {
		if p.Conns[i] != nil {
			fmt.Printf("\t client id=%d, ip=%s\n", i, (*p.Conns[i]).RemoteAddr())
		}
	}
}

func (p *ProxyTCP) Close() {
	for i := 0; i < Conf.NClients; i++ {
		if p.Conns[i] != nil {
			_ = (*p.Conns[i]).Close()
		}
	}
	close(p.Done)
	_ = p.Listener.Close()
	p.Wg.Wait()
}
