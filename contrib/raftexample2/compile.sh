cd ~/go/src/etcd/contrib/raftexample2
rm -rf raftserver raftclient raftexample*
go build -o raftserver ./server
go build -o raftclient ./client