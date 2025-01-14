package shardctrler

import (
	"log"
	"time"
)

//
// Shard controller: assigns shards to replication groups.
//
// RPC interface:
// Join(servers) -- add a set of groups (gid -> server-list mapping).
// Leave(gids) -- delete a set of groups.
// Move(shard, gid) -- hand off one shard from current owner to gid.
// Query(num) -> fetch Config # num, or latest config if num==-1.
//
// A Config (configuration) describes a set of replica groups, and the
// replica group responsible for each shard. Configs are numbered. Config
// #0 is the initial configuration, with no groups and all shards
// assigned to group 0 (the invalid group).
//
// You will need to add fields to the RPC argument structs.
//

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

// The number of shards.
const NShards = 10
const Timeout = 500 * time.Millisecond

// A configuration -- an assignment of shards to groups.
// Please don't change this.
type Config struct {
	Num    int              // config number
	Shards [NShards]int     // shard -> gid
	Groups map[int][]string // gid -> servers[]
}

func (c *Config) Clone() *Config {
	clone := &Config{
		Num:    c.Num,
		Shards: c.Shards,
		Groups: make(map[int][]string),
	}
	for k, v := range c.Groups {
		clone.Groups[k] = v
	}
	return clone
}

func DefaultConfig() Config {
	return Config{
		Groups: make(map[int][]string),
	}
}

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
	ErrTimeout     = "ErrTimeout"
)

type Err string
type Operation uint8

const (
	JoinOp Operation = iota
	LeaveOp
	MoveOp
	QueryOp
)

type Op struct {
	// Your data here.
	Servers map[int][]string // new GID -> servers mappings
	GIDs    []int
	Shard   int
	GID     int
	Num     int

	// Operation type
	Operation Operation
	ClientId  int64
	RequestId int64
}

type OperationReply struct {
	Err              Err
	ControllerConfig Config
}

type LastOperation struct {
	RequestId int64
	Reply     *OperationReply
}

type JoinArgs struct {
	Servers map[int][]string // new GID -> servers mappings

	ClientId  int64
	RequestId int64
}

type JoinReply struct {
	WrongLeader bool
	Err         Err
}

type LeaveArgs struct {
	GIDs []int

	ClientId  int64
	RequestId int64
}

type LeaveReply struct {
	WrongLeader bool
	Err         Err
}

type MoveArgs struct {
	Shard int
	GID   int

	ClientId  int64
	RequestId int64
}

type MoveReply struct {
	WrongLeader bool
	Err         Err
}

type QueryArgs struct {
	Num int // desired config number
}

type QueryReply struct {
	WrongLeader bool
	Err         Err
	Config      Config
}
