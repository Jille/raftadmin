# raftadmin

RaftAdmin is a gRPC service to invoke methods on https://godoc.org/github.com/hashicorp/raft#Raft. It only works with [Hashicorp's Raft implementation](https://github.com/hashicorp/raft).

## Usage

```go
// ...
r, err := raft.NewRaft(...)
s := grpc.NewServer()
raftadmin.Register(s, r)
// ...
```

Adding the call to `raftadmin.Register` will register a new gRPC service on your existing server that exposes a bunch of methods so they can be called remotely.

For example, I use this to add servers (voters) after initial bootstrap.

## Invocations

```shell
$ raftadmin
Usage: raftadmin <host:port> <command> <args...>
Commands: add_nonvoter, add_voter, applied_index, apply_log, await, barrier, demote_voter, forget, get_configuration, last_contact, last_index, leader, leadership_transfer, leadership_transfer_to_server, remove_server, shutdown, snapshot, state, stats, verify_leader

$ raftadmin 127.0.0.1:50051 add_voter serverb 127.0.0.1:50052 0
Invoking AddVoter(id: "serverb" address: "127.0.0.1:50052")
Response: operation_token:  "4a86d2efa417af281ac540bfede8fcb735e0b224"
Invoking Await(id: "serverb" address: "127.0.0.1:50052")
Response: index:  3
```

## Raw calls

```shell
$ grpc_cli call 127.0.0.1:50051 RaftAdmin.AddVoter 'id: "serverb" address: "127.0.0.1:50052"'
operation_token: "c496024e534e7bb11327f7aa48d6aed106a89a76"
Rpc succeeded with OK status
$ grpc_cli call 127.0.0.1:50051 RaftAdmin.Await 'operation_token: "c496024e534e7bb11327f7aa48d6aed106a89a76"'
index: 7
Rpc succeeded with OK status
$ grpc_cli call 127.0.0.1:50051 RaftAdmin.Forget 'operation_token: "c496024e534e7bb11327f7aa48d6aed106a89a76"'
Rpc succeeded with OK status
```

AddVoter starts a new raft operation and returns once it is enqueued. It returns an operation_token with which you can call Await. Nearly all errors are detected by Await and returns as AwaitResponse.error.

Last, call Forget to make the server forget the operation token and free up the memory.

## Missing methods

* AddPeer/RemovePeer are deprecated in raft.
* Snapshot doesn't return any information about the snapshot.
* Apply because it's a thin wrapper around ApplyLog.
* RegisterObserver/DeregisterObserver because I was lazy.
* BootstrapCluster and Restore because they are dangerous.
