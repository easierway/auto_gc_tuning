// The project is to automate the adjustment of GOGC parameter
// Author: chaocai2001@icloud.com
// 2021.3
package autotuning

import (
	"io/ioutil"
	"math"
	"sync/atomic"

	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	mem_util "github.com/shirou/gopsutil/mem"
)

var startTime time.Time

var gTuningParam TuningParam
var gStartingTimeSpentMins float64

var nextGOGC = 100

var LastForceGCNum = uint32(0)

var tunerStartTime time.Time

var targetNextGC float64
var lastReadingMemTime time.Time

var gIsHeapStable bool

const SynIntervalMins = time.Minute * 3
const TuningStep = 10
const MinIntervalMs = 200
const RamThresholdInPercentage = float32(80)
const cgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
const MaxMemPercent = float32(85)
const MaxMemReadingIntervalMins = time.Minute * 1

var gTuningParamCache = &tuningParamCache{}

type tuningParamCache struct {
	tuningParam atomic.Value
}

func (cache *tuningParamCache) put(tuningParam TuningParam) {
	cache.tuningParam.Store(tuningParam)
}

func (cache *tuningParamCache) get() TuningParam {
	ret, isOk := cache.tuningParam.Load().(TuningParam)
	if !isOk {
		panic("wrong data in tuning param cache.")
	}
	return ret
}

// UpdateTuningParam is to update the tuning param at runtime.
func UpdateTuningParam(param TuningParam) {
	gTuningParamCache.put(param)
}

func syncTuningParam() {

	for {
		time.Sleep(SynIntervalMins)
		updateTuningParam(gTuningParamCache.get())
	}
}

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

var heapInuse uint64

func needToReadMem() bool {
	if !gIsHeapStable {
		return true
	}

	if time.Since(startTime) < time.Minute*time.Duration(gStartingTimeSpentMins) {
		return true
	}
	println("MaxReadingInterval", time.Since(lastReadingMemTime) > MaxMemReadingIntervalMins)
	if time.Since(lastReadingMemTime) > MaxMemReadingIntervalMins {
		return true
	}
	return false
}

func tuningGOGC() {
	if needToReadMem() {
		println("read memstate")
		lastReadingMemTime = time.Now()
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
	} else {
		println("not read memstate")
	}
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
	IsToOutputDebugInfo                    bool    // whether to output the debug infoΩ
}

func updateTuningParam(param TuningParam) {
	gTuningParam = param
	nextGOGC = param.LowestGOGC
	targetNextGC = totalMem * param.PropertionActiveHeapSizeInTotalMemSize
}

// NewTuner is to create a tuner for tuning GOGC
// useCgroup : when your program is running in Docker env/with cgroup configuration
func NewTunerExt(useCgroup bool, param TuningParam,
	isHeapStable bool, startingTimeSpentMins int64) *finalizer {
	gIsHeapStable = isHeapStable
	gStartingTimeSpentMins = float64(startingTimeSpentMins)
	var err error
	if useCgroup {
		totalMem, err = getCGroupMemoryLimit()
	} else {
		totalMem, err = getMachineMemoryLimit()
	}
	if err != nil {
		panic(err)
	}
	gTuningParamCache.put(param)
	updateTuningParam(param)
	go syncTuningParam()
	startTime = time.Now()
	lastReadingMemTime = time.Now()
	f := &finalizer{
		ch: make(chan time.Time, 1),
	}
	f.ref = &finalizerRef{parent: f}
	runtime.SetFinalizer(f.ref, finalizerHandler)
	f.ref = nil

	return f
}

func NewTuner(useCgroup bool, param TuningParam,
) *finalizer {
	return NewTunerExt(useCgroup, param, false, 0)
}
