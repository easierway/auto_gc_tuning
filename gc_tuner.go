// The project is to automate the adjustment of GOGC parameter
// Author: chaocai2001@icloud.com
// 2021.3
package autotuning

import (
	"io/ioutil"
	"math"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	mem_util "github.com/shirou/gopsutil/mem"
)

var gTuningParam TuningParam

var nextGOGC = 100

var LastForceGCNum = uint32(0)

const TuningStep = 10

const RamThresholdInPercentage = float32(80)
const cgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"

type finalizer struct {
	ch  chan time.Time
	ref *finalizerRef
}

type finalizerRef struct {
	parent *finalizer
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

var (
	m         runtime.MemStats
	heapInuse float64
	totalMem  float64
)

func parseUint(s string, base, bitSize int) (uint64, error) {
	v, err := strconv.ParseUint(s, base, bitSize)
	if err != nil {
		intValue, intErr := strconv.ParseInt(s, base, bitSize)
		if intErr == nil && intValue < 0 {
			return 0, nil
		} else if intErr != nil &&
			intErr.(*strconv.NumError).Err == strconv.ErrRange &&
			intValue < 0 {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

func readUint(path string) (uint64, error) {
	v, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return parseUint(strings.TrimSpace(string(v)), 10, 64)
}

func getCGroupMemoryLimit() (float64, error) {
	usage, err := readUint(cgroupMemLimitPath)
	if err != nil {
		return 0, err
	}
	machineMemory, err := mem_util.VirtualMemory()
	if err != nil {
		return 0, err
	}
	limit := math.Min(float64(usage), float64(machineMemory.Total))
	return limit, nil
}

func getMachineMemoryLimit() (float64, error) {
	machineMemory, err := mem_util.VirtualMemory()
	if err != nil {
		return 0, err
	}
	limit := float64(machineMemory.Total)
	return limit, nil
}

func tuningGOGC() {
	runtime.ReadMemStats(&m)
	heapInuse = float64(m.HeapInuse)
	if float64(m.NextGC) < totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize &&
		float64(m.NextGC) > totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize*0.8 {
		if gTuningParam.IsToOutputDebugInfo {
			println("skip the tuning")
			println("Heap In Use", bToMb(m.HeapInuse))
			println("NextGC Size", bToMb(m.NextGC))
			println("Limit size", bToMb(uint64(totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize)))
			println("nextGOGC", nextGOGC)
		}
		return
	}
	if float64(m.NextGC) > totalMem {
		if nextGOGC > gTuningParam.LowestGOGC {
			nextGOGC = nextGOGC - 2*TuningStep
		}
		if gTuningParam.IsToOutputDebugInfo {
			println("Heap In Use", bToMb(m.HeapInuse))
			println("NextGC Size", bToMb(m.NextGC))
			println("nextGOGC", nextGOGC)
			println("Limit size", bToMb(uint64(totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize)))
		}
		debug.SetGCPercent(nextGOGC)
		return
	}

	if (float64(m.NextGC) < totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize) &&
		((nextGOGC + TuningStep) < gTuningParam.HighestGOGC) {
		nextGOGC = nextGOGC + TuningStep
	}
	if gTuningParam.IsToOutputDebugInfo {
		println("Heap In Use", bToMb(m.HeapInuse))
		println("nextGOGC", nextGOGC)
		println("NextGC Size", bToMb(m.NextGC))
		println("Limit size", bToMb(uint64(totalMem*gTuningParam.PropertionActiveHeapSizeInTotalMemSize)))
	}
	debug.SetGCPercent(nextGOGC)
}

func finalizerHandler(f *finalizerRef) {
	select {
	case f.parent.ch <- time.Time{}:
	default:
	}
	tuningGOGC()
	runtime.SetFinalizer(f, finalizerHandler)
}

// TuningParam
type TuningParam struct {
	LowestGOGC                             int     // the lowest value of GOGC
	HighestGOGC                            int     // the highest value of GOGC (define the scope for tuning)
	PropertionActiveHeapSizeInTotalMemSize float64 // the value of (HeapInUse/MemoryLimit), the value could be larger than 1
	IsToOutputDebugInfo                    bool    // whether to output the debug info
}

// NewTuner is to create a tuner for tuning GOGC
// useCgroup : when your program is running in Docker env/with cgroup configuration
func NewTuner(useCgroup bool, param TuningParam) *finalizer {

	var err error
	if useCgroup {
		totalMem, err = getCGroupMemoryLimit()
	} else {
		totalMem, err = getMachineMemoryLimit()
	}
	if err != nil {
		panic(err)
	}
	gTuningParam = param
	nextGOGC = param.LowestGOGC
	f := &finalizer{
		ch: make(chan time.Time, 1),
	}

	f.ref = &finalizerRef{parent: f}
	runtime.SetFinalizer(f.ref, finalizerHandler)
	f.ref = nil
	return f
}
