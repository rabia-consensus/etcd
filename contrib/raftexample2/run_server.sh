cd ~/go/src/etcd/contrib/raftexample2
./raftserver --id 1 --cluster http://localhost:12379,http://localhost:22379,http://localhost:32379 --proxy localhost:12380 &
./raftserver --id 2 --cluster http://localhost:12379,http://localhost:22379,http://localhost:32379 --proxy localhost:22380 &
./raftserver --id 3 --cluster http://localhost:12379,http://localhost:22379,http://localhost:32379 --proxy localhost:32380 &