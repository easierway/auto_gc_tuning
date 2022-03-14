// The project is to automate the adjustment of GOGC parameter
// Author: chaocai2001@icloud.com
// 2021.3
package autotuning

import (
	"io/ioutil"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	mem_util "github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/process"
)

var gTuningParam TuningParam

var nextGOGC = 100

var LastForceGCNum = uint32(0)

var tunerStartTime time.Time

var targetNextGC float64

const TuningStep = 10
const MinIntervalMs = 200
const RamThresholdInPercentage = float32(80)
const cgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
const MaxMemPercent = float32(85)

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
	m        runtime.MemStats
	totalMem float64
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

func trigerOOM_Protection() bool {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		println("failed to get process info")
		return false
	}

	mem, err := p.MemoryPercent()
	if err != nil {
		println("failed to get mem info")
		return false
	}
	if mem > MaxMemPercent {
		println("reach to the high memory limit, trigger the force GC")
		runtime.GC()
		return true
	}
	return false

}

var heapInuse uint64

func tuningGOGC() {
	runtime.ReadMemStats(&m)

	heapInuse = m.HeapInuse

	if bToMb(heapInuse) < 1 {
		heapInuse = 10 * 1024 * 1024
	}

	nextGOGC = int((targetNextGC/float64(heapInuse) - 1) * 100)
	if nextGOGC < gTuningParam.LowestGOGC {
		nextGOGC = gTuningParam.LowestGOGC
	}
	if nextGOGC > gTuningParam.HighestGOGC {
		nextGOGC = gTuningParam.HighestGOGC
		println("the highest GOGC seems low.")
	}
	debug.SetGCPercent(nextGOGC)

	if gTuningParam.IsToOutputDebugInfo {
		println("heap in use", bToMb(m.HeapInuse))
		println("target GC size", bToMb(uint64(targetNextGC)))
		println("nextGOGC", nextGOGC)
	}
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
	targetNextGC = totalMem * param.PropertionActiveHeapSizeInTotalMemSize
	f.ref = &finalizerRef{parent: f}
	runtime.SetFinalizer(f.ref, finalizerHandler)
	f.ref = nil
	return f
}
