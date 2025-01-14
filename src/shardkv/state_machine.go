package shardkv

type MemoryKVStateMachine struct {
	KV     map[string]string
	Status ShardStatus
}

func NewMemoryKVStateMachine() *MemoryKVStateMachine {
	return &MemoryKVStateMachine{
		KV:     make(map[string]string),
		Status: Normal,
	}
}

func (mkv *MemoryKVStateMachine) Clone() *MemoryKVStateMachine {
	newMkv := NewMemoryKVStateMachine()
	for k, v := range mkv.KV {
		newMkv.KV[k] = v
	}
	newMkv.Status = mkv.Status
	return newMkv
}

func (mkv *MemoryKVStateMachine) CopyData() map[string]string {
	data := make(map[string]string)
	for k, v := range mkv.KV {
		data[k] = v
	}
	return data
}

func (mkv *MemoryKVStateMachine) Get(key string) (string, Err) {
	if value, ok := mkv.KV[key]; ok {
		return value, OK
	}
	return "", ErrNoKey
}

func (mkv *MemoryKVStateMachine) Put(key, value string) Err {
	mkv.KV[key] = value
	return OK
}

func (mkv *MemoryKVStateMachine) Append(key, value string) Err {
	mkv.KV[key] += value
	return OK
}
