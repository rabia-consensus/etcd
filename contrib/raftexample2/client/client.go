package main

import (
	"flag"
	"fmt"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/config"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/msg"
	"go.etcd.io/etcd/v3/contrib/raftexample2/rstring"
	"go.etcd.io/etcd/v3/contrib/raftexample2/system"
	"go.etcd.io/etcd/v3/contrib/raftexample2/tcp"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

var clientIdx = flag.Uint("idx", 0, "0, 1, 2, ...")
var proxyAddr = flag.String("proxy", "localhost:23030", "[proxy's private ip]")

type BatchedCmdLog struct {
	SendTime    time.Time     // the send time of this client-batched command
	ReceiveTime time.Time     // the receive time of client-batched command
	Duration    time.Duration // the calculate latency of this command (ReceiveTime - SendTime)
}

type Client struct {
	ClientId uint32
	Wg       *sync.WaitGroup // todo
	Done     chan struct{}   // todo

	TCP  *tcp.ClientTCP
	Rand *rand.Rand

	CommandLog                             []BatchedCmdLog
	SentSoFar, ReceivedSoFar               int
	startSending, endSending, endReceiving time.Time
}

func ClientInit() *Client {
	return &Client{
		ClientId: uint32(*clientIdx),
		Wg:       &sync.WaitGroup{},
		Done:     make(chan struct{}),

		TCP:  tcp.ClientTcpInit(uint32(*clientIdx), *proxyAddr),
		Rand: rand.New(rand.NewSource(time.Now().UnixNano() * int64(*clientIdx))),

		CommandLog: make([]BatchedCmdLog, Conf.NClientRequests/Conf.ClientBatchSize),
	}
}

func (c *Client) Prologue() {
	go system.SigListen(c.Done) //
	c.TCP.Connect()
	go c.terminalLogger()
}

func (c *Client) Epilogue() {
	close(c.Done)
	c.TCP.Close()
}

func (c *Client) CloseLoopClient() {
	c.startSending = time.Now()
	ticker := time.NewTicker(Conf.ClientTimeout)
MainLoop:
	for i := 0; i < Conf.NClientRequests/Conf.ClientBatchSize; i++ {
		c.sendOneRequest(i)
		select {
		//case rep := <-c.TCP.RecvChan:
		//	c.processOneReply(rep)
		case <-ticker.C:
			break MainLoop
		default:

		}
	}
	c.endSending = time.Now()
	c.endReceiving = time.Now()
}

func (c *Client) OpenLoopClient() {
	c.Wg.Add(2)
	go func() {
		c.startSending = time.Now()
		for i := 0; i < Conf.NClientRequests/Conf.ClientBatchSize; i++ {
			if c.SentSoFar-c.ReceivedSoFar >= 10000*Conf.ClientBatchSize {
				time.Sleep(500 * time.Millisecond)
				i--
				continue
			}
			c.sendOneRequest(i)
		}
		fmt.Println(c.ClientId, "client requests all sent")
		c.endSending = time.Now()
		c.Wg.Done()
	}()
	go func() {
		for i := 0; i < Conf.NClientRequests/Conf.ClientBatchSize; i++ {
			rep := <-c.TCP.RecvChan
			c.processOneReply(rep)
		}
		c.endReceiving = time.Now()
		c.Wg.Done()
	}()
	c.Wg.Wait()
}

func (c *Client) sendOneRequest(i int) {
	obj := Command{CliId: c.ClientId, CliSeq: uint32(i), Commands: make([]string, Conf.ClientBatchSize)}
	for j := 0; j < Conf.ClientBatchSize; j++ {
		val := fmt.Sprintf("%d%v%v", c.Rand.Intn(2),
			rstring.RandString(c.Rand, Conf.KeyLen),
			rstring.RandString(c.Rand, Conf.ValLen))
		obj.Commands[j] = val
	}

	time.Sleep(time.Duration(Conf.ClientThinkTime) * time.Millisecond)

	c.CommandLog[i].SendTime = time.Now()
	c.TCP.SendChan <- obj
	c.SentSoFar += Conf.ClientBatchSize
}

func (c *Client) processOneReply(rep Command) {
	if c.CommandLog[rep.CliSeq].Duration != time.Duration(0) {
		panic("already received")
	}
	c.CommandLog[rep.CliSeq].ReceiveTime = time.Now()
	c.CommandLog[rep.CliSeq].Duration = c.CommandLog[rep.CliSeq].ReceiveTime.Sub(c.CommandLog[rep.CliSeq].SendTime)
	c.ReceivedSoFar += Conf.ClientBatchSize
}

func (c *Client) terminalLogger() {
	ticker := time.NewTicker(Conf.ClientLogInterval)

	lastRecv, thisRecv := 0, 0
	for {
		select {
		case <-c.Done:
			return
		case <-ticker.C:
			thisRecv = c.ReceivedSoFar
			tho := math.Round(float64(thisRecv-lastRecv) / Conf.ClientLogInterval.Seconds())
			log.Printf("Client ClientId=%d, Sent=%d, Recv=%d, Interval Recv Tput (cmd/sec)=%f",
				c.ClientId, c.SentSoFar, thisRecv, tho)
			lastRecv = thisRecv
		}
	}
}

func main() {
	flag.Parse()
	Conf.LoadConfigs()

	cli := ClientInit()
	log.Printf("raft client %d starts, connect to proxy %s", cli.ClientId, *proxyAddr)

	cli.Prologue()
	if Conf.ClosedLoop {
		cli.CloseLoopClient()
	} else {
		cli.OpenLoopClient()
	}
	cli.Epilogue()
}
