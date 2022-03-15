# GOGC Auto-Tuning
GOGC is the only parameter to tune the Golang's GC.
This project is to automate the tuning of the parameter.

Example:
```
func init() {
	NewTuner(false, //when your program is running in Docker env/with cgroup configuration
	TuningParam{
		LowestGOGC:                             100, // LowestGOGC & HighestGOGC is to define the scope of tuning
		HighestGOGC:                            1000, 
		PropertionActiveHeapSizeInTotalMemSize: float64(0.9), //The value of (HeapInUse/MemoryLimit), the value could be larger than 1
		IsToOutputDebugInfo:                    false, //set it false, when running in prod
		SwapRatio:                              0.3,  // the recommend value is [0, 0.5]
	})
	
}
```

Any problem or suggestion, please contact chaocai2001@icloud.com