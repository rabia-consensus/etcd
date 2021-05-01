package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/config"
	. "go.etcd.io/etcd/v3/contrib/raftexample2/msg"
	"go.etcd.io/etcd/v3/contrib/raftexample2/tcp"
	"log"
	"sync"
)

type kv struct {
	Key string
	Val string
}

type Proxy struct {
	SvrId uint32
	Wg    *sync.WaitGroup
	mu    sync.RWMutex

	ClientsIn chan Command
	proposeC  chan<- string
	commitC   <-chan *commit
	errorC    <-chan error

	TCP *tcp.ProxyTCP

	KVStore     map[string]string
	snapshotter *snap.Snapshotter

	CurrDec   *ConsensusObj // current decision
	CurrInsId int
	CurrSeq   uint32
}

func ProxyInit(svrId uint32, doneWg *sync.WaitGroup, proxyIp string, toProxy chan Command,
	proposeC chan<- string, commitC <-chan *commit, errorC <-chan error, snapshotter *snap.Snapshotter) *Proxy {
	return &Proxy{
		SvrId: svrId,
		Wg:    doneWg,

		ClientsIn:   toProxy,
		proposeC:    proposeC,
		commitC:     commitC,
		errorC:      errorC,
		snapshotter: snapshotter,

		TCP:     tcp.ProxyTcpInit(svrId, proxyIp, toProxy),
		KVStore: make(map[string]string),
	}
}

func (p *Proxy) Prologue() {
	p.TCP.Connect()
}

func (p *Proxy) Epilogue() {
	p.TCP.Close()
}

func (p *Proxy) CmdReceiver() {
	defer p.Wg.Done()

MainLoop:
	for {
		select {

		case <-p.errorC:
			break MainLoop

		case msg := <-p.ClientsIn: // a client's request object
			for _, cmd := range msg.Commands {
				fmt.Println("about to propose", cmd[1:1+Conf.KeyLen], cmd[1+Conf.KeyLen:])
				p.Propose(cmd[1:1+Conf.KeyLen], cmd[1+Conf.KeyLen:])
			}
		}
	}

}

func (p *Proxy) Propose(k, v string) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(kv{k, v}); err != nil {
		log.Fatal(err)
	}
	p.proposeC <- buf.String()
}

func (p *Proxy) readCommits(commitC <-chan *commit, errorC <-chan error) {
	defer p.Wg.Done()

	cnt := 0
	for commit := range commitC {
		//fmt.Println("here 1")
		for _, data := range commit.data {
			//fmt.Println("here 2")
			var dataKv kv
			dec := gob.NewDecoder(bytes.NewBufferString(data))
			if err := dec.Decode(&dataKv); err != nil {
				log.Fatalf("raftexample: could not decode message (%v)", err)
			}
			p.mu.Lock()
			p.KVStore[dataKv.Key] = dataKv.Val
			p.mu.Unlock()
			fmt.Println(cnt, "readCommits", dataKv.Key, dataKv.Val)
			cnt++
		}
		close(commit.applyDoneC)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (s *Proxy) getSnapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.KVStore)
}

func (s *Proxy) loadSnapshot() (*raftpb.Snapshot, error) {
	snapshot, err := s.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Proxy) recoverFromSnapshot(snapshot []byte) error {
	var store map[string]string
	if err := json.Unmarshal(snapshot, &store); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.KVStore = store
	return nil
}
