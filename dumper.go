package connpool

type Dumper interface {
	Dump()
}

type PoolInfo struct {
}

var poolInfo *PoolInfo

func initPoolInfo() {
	poolInfo = &PoolInfo{}
}

func (pi *PoolInfo) Clone() PoolInfo {
	return PoolInfo{}
}
