package main

import (
	"flag"
	"go.etcd.io/etcd/raft/v3/raftpb"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/config"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/msg"
	"log"
	"strings"
	"sync"
)

func main() {
	cluster := flag.String("cluster", "http://127.0.0.1:9021", "comma separated cluster peers")
	id := flag.Uint("id", 1, "node ID")
	proxyAddr := flag.String("proxy", "localhost:9120", "key-value server port")
	join := flag.Bool("join", false, "join an existing cluster")
	flag.Parse()
	Conf.LoadConfigs()

	proposeC := make(chan string, Conf.LenChannel) //todo: channel size
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	// raft provides a commit stream for the proposals from the http api
	var proxy *Proxy
	getSnapshot := func() ([]byte, error) { return proxy.getSnapshot() }
	commitC, errorC, snapshotterReady := newRaftNode(int(*id), strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC)

	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	cmds := make(chan Command)
	proxy = ProxyInit(uint32(*id), wg, *proxyAddr, cmds, proposeC, commitC, errorC, <-snapshotterReady)

	proxy.Prologue()
	wg.Add(2)
	go proxy.CmdReceiver()
	go proxy.readCommits(commitC, errorC)

	// exit when raft goes down
	if err, ok := <-errorC; ok {
		close(done)
		wg.Wait()
		log.Fatal(err)
	}
}
