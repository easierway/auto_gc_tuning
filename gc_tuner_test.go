package autotuning

import (
	//"runtime/debug"
	"fmt"
	//"runtime"
	"os"
	"testing"
	"time"
)

func alloc() *[]int64 {
	b := make([]int64, 100000000)
	time.Sleep(time.Millisecond * 1)
	return &b
}

var ballast []byte

func TestTuner(t *testing.T) {
	//bb := make([]*[]int64, 10)
	NewTuner(false, TuningParam{
		LowestGOGC:                             1,
		HighestGOGC:                            10000000,
		PropertionActiveHeapSizeInTotalMemSize: float64(0.7),
		IsToOutputDebugInfo:                    true, // set it false, when running in prod
		SwapRatio:                              0.5,  // the recommend value is [0, 0.5]
	})
	//debug.SetGCPercent(200)

	fmt.Println(int32(os.Getpid()))
	for {
		//	bb = append(bb, alloc())
		_ = alloc()
	}
}
