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
	NewTunerExt(false, TuningParam{
		LowestGOGC:                             1,
		HighestGOGC:                            10000000,
		PropertionActiveHeapSizeInTotalMemSize: float64(0.7),
		IsToOutputDebugInfo:                    true, // set it false, when running in prod
	}, true, 1)
	//debug.SetGCPercent(200)
	go func() {

		time.Sleep(time.Second * 10)
		fmt.Println("reset param")
		UpdateTuningParam(TuningParam{
			LowestGOGC:                             100,
			HighestGOGC:                            500,
			PropertionActiveHeapSizeInTotalMemSize: float64(1),
			IsToOutputDebugInfo:                    true,
		})
	}()
	fmt.Println(int32(os.Getpid()))
	for {
		//	bb = append(bb, alloc())
		_ = alloc()
	}
}
