# GOGC Auto-Tuning
GOGC is the only parameter to tune the Golang's GC.
This project is to automate the tuning of the parameter.

Example:
```
func init() {
	NewTuner(false, //when your program is running in Docker env/with cgroup configuration
	TuningParam{
		LowestGOGC:                             10, // I recommend set the value as lower as possible, but make it larger than 0. Let this program control it, otherwise it tends to OOM.
		HighestGOGC:                            1000, // The value can be set very large, the program will control it reasonable
		PropertionActiveHeapSizeInTotalMemSize: float64(1.3), //Recommand value is [1.0-1.5]. The value of (HeapInUse/MemoryLimit), the value could be larger than 1
		IsToOutputDebugInfo:                    false, //set it false, when running in prod
	})
	
}
```

You can update the tuning parameters at runtime by invoking the following API.
(The updating would be put into effect after 3-6 minutes)

```
UpdateTuningParam(param TuningParam)
```

Any problem or suggestion, please contact chaocai2001@icloud.com