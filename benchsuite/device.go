package benchsuite

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// CollectDeviceProfile reads the device description. Fields that cannot be
// read stay empty or zero.
func CollectDeviceProfile() DeviceProfile {
	profile := DeviceProfile{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPUCores:   runtime.NumCPU(),
		CPUMaxMHz:  cpuMaxMHz(),
		RAMTotalMB: ramTotalMB(),
	}
	readAndroidProperties(&profile)
	return profile
}

// cpuMaxMHz is the highest cpuinfo_max_freq, which the kernel gives in kHz.
func cpuMaxMHz() int {
	paths, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/cpuinfo_max_freq")
	highest := 0
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if khz, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			highest = max(highest, khz/1000)
		}
	}
	return highest
}

// ramTotalMB is MemTotal from /proc/meminfo.
func ramTotalMB() int {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.Atoi(fields[1])
			return kb / 1024
		}
	}
	return 0
}
