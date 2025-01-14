package shardkv

import (
	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
	"6.5840/shardctrler"
	"bytes"
	"sync"
	"sync/atomic"
	"time"
)

type ShardKV struct {
	mu           sync.Mutex
	me           int
	rf           *raft.Raft
	applyCh      chan raft.ApplyMsg
	make_end     func(string) *labrpc.ClientEnd
	gid          int
	ctrlers      []*labrpc.ClientEnd
	maxraftstate int // snapshot if log grows this big
	persister    *raft.Persister

	// Your definitions here.
	dead           int32 // set by Kill()
	lastApplied    int
	shards         map[int]*MemoryKVStateMachine
	notifyChans    map[int]chan *OpReply
	duplicateTable map[int64]*LastOperation
	currentConfig  shardctrler.Config
	preConfig      shardctrler.Config
	mck            *shardctrler.Clerk
}

func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	kv.mu.Lock()
	if !kv.matchGroup(args.Key) {
		reply.Err = ErrWrongGroup
		kv.mu.Unlock()
		return
	}
	kv.mu.Unlock()

	index, _, isLeader := kv.rf.Start(RaftCommand{
		CommandType: ClientOperation,
		Command: Op{
			Key:    args.Key,
			OpType: GetOp,
		}})
	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}

	kv.mu.Lock()
	notifyCh := kv.getNotifyChan(index)
	kv.mu.Unlock()

	select {
	case opReply := <-notifyCh:
		reply.Err = opReply.Err
		reply.Value = opReply.Value
	case <-time.After(ClientRequestTimeout):
		reply.Err = ErrTimeout
	}

	go func() {
		kv.mu.Lock()
		delete(kv.notifyChans, index)
		kv.mu.Unlock()
	}()
}

func (kv *ShardKV) matchGroup(key string) bool {
	shard := key2shard(key)
	gid := kv.currentConfig.Shards[shard]
	shardStatus := kv.shards[shard].Status
	return gid == kv.gid && (shardStatus == Normal || shardStatus == GC)
}

func (kv *ShardKV) getNotifyChan(index int) chan *OpReply {
	notifyCh, ok := kv.notifyChans[index]
	if !ok {
		notifyCh = make(chan *OpReply, 1)
		kv.notifyChans[index] = notifyCh
	}
	return notifyCh
}

func (kv *ShardKV) isDuplicate(clientId int64, requestId int64) bool {
	lastOp, ok := kv.duplicateTable[clientId]
	if !ok {
		return false
	}
	return lastOp.RequestId >= requestId
}

func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	kv.mu.Lock()

	if !kv.matchGroup(args.Key) {
		reply.Err = ErrWrongGroup
		kv.mu.Unlock()
		return
	}

	if kv.isDuplicate(args.ClientId, args.RequestId) {
		opReply := kv.duplicateTable[args.ClientId].Reply
		reply.Err = opReply.Err
		kv.mu.Unlock()
		return
	}
	kv.mu.Unlock()

	index, _, isLeader := kv.rf.Start(RaftCommand{
		CommandType: ClientOperation,
		Command: Op{
			Key:       args.Key,
			Value:     args.Value,
			OpType:    getOpType(args.Op),
			ClientId:  args.ClientId,
			RequestId: args.RequestId,
		},
	})

	if !isLeader {
		reply.Err = ErrWrongLeader
		return
	}

	kv.mu.Lock()
	notifyCh := kv.getNotifyChan(index)
	kv.mu.Unlock()

	select {
	case opReply := <-notifyCh:
		reply.Err = opReply.Err
	case <-time.After(ClientRequestTimeout):
		reply.Err = ErrTimeout
	}

	go func() {
		kv.mu.Lock()
		delete(kv.notifyChans, index)
		kv.mu.Unlock()
	}()
}

// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (kv *ShardKV) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *ShardKV) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// servers[] contains the ports of the servers in this group.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
//
// the k/v server should snapshot when Raft's saved state exceeds
// maxraftstate bytes, in order to allow Raft to garbage-collect its
// log. if maxraftstate is -1, you don't need to snapshot.
//
// gid is this group's GID, for interacting with the shardctrler.
//
// pass ctrlers[] to shardctrler.MakeClerk() so you can send
// RPCs to the shardctrler.
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs. You'll need this to send RPCs to other groups.
//
// look at client.go for examples of how to use ctrlers[]
// and make_end() to send RPCs to the group owning a specific shard.
//
// StartServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int, gid int, ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})
	labgob.Register(RaftCommand{})
	labgob.Register(shardctrler.Config{})
	labgob.Register(ShardOperationArgs{})
	labgob.Register(ShardOperationReply{})

	kv := new(ShardKV)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.make_end = make_end
	kv.gid = gid
	kv.ctrlers = ctrlers
	kv.persister = persister

	// Your initialization code here.

	// Use something like this to talk to the shardctrler:

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	kv.dead = 0
	kv.lastApplied = 0
	kv.shards = make(map[int]*MemoryKVStateMachine)
	kv.notifyChans = make(map[int]chan *OpReply)
	kv.duplicateTable = make(map[int64]*LastOperation)
	kv.currentConfig = shardctrler.DefaultConfig()
	kv.preConfig = shardctrler.DefaultConfig()

	kv.mck = shardctrler.MakeClerk(kv.ctrlers)

	kv.restoreFromSnapshot(persister.ReadSnapshot())

	go kv.applyTask()
	go kv.fetchConfigTask()
	go kv.shardMigrationTask()
	go kv.shardGCTask()
	//go kv.snapshotor()

	return kv
}

func (kv *ShardKV) applyToStateMachine(op Op, shardId int) *OpReply {
	var value string
	var err Err
	switch op.OpType {
	case GetOp:
		value, err = kv.shards[shardId].Get(op.Key)
	case PutOp:
		err = kv.shards[shardId].Put(op.Key, op.Value)
	case AppendOp:
		err = kv.shards[shardId].Append(op.Key, op.Value)
	default:
		panic("Invalid operation type")
	}
	return &OpReply{Value: value, Err: err}
}

func (kv *ShardKV) makeSnapshot(index int) {
	buf := new(bytes.Buffer)
	enc := labgob.NewEncoder(buf)
	_ = enc.Encode(kv.shards)
	_ = enc.Encode(kv.duplicateTable)
	_ = enc.Encode(kv.currentConfig)
	_ = enc.Encode(kv.preConfig)
	kv.rf.Snapshot(index, buf.Bytes())
}

func (kv *ShardKV) restoreFromSnapshot(snapshot []byte) {
	if snapshot == nil || len(snapshot) < 1 {
		for i := 0; i < shardctrler.NShards; i++ {
			if _, ok := kv.shards[i]; !ok {
				kv.shards[i] = NewMemoryKVStateMachine()
			}
		}
		return
	}

	buf := bytes.NewBuffer(snapshot)
	dec := labgob.NewDecoder(buf)

	var shards map[int]*MemoryKVStateMachine
	var duplicateTable map[int64]*LastOperation
	var currentConfig shardctrler.Config
	var preConfig shardctrler.Config

	if dec.Decode(&shards) != nil || dec.Decode(&duplicateTable) != nil || dec.Decode(&currentConfig) != nil || dec.Decode(&preConfig) != nil {
		panic("Failed to decode snapshot")
	}

	kv.shards = shards
	kv.duplicateTable = duplicateTable
	kv.currentConfig = currentConfig
	kv.preConfig = preConfig
}
