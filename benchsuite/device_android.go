package benchsuite

import (
	"os/exec"
	"strconv"
	"strings"
)

// readAndroidProperties fills the Android fields from system properties.
// Only build and product properties are read, never a serial number.
func readAndroidProperties(profile *DeviceProfile) {
	profile.AndroidRelease = systemProperty("ro.build.version.release")
	profile.AndroidSdk, _ = strconv.Atoi(systemProperty("ro.build.version.sdk"))
	profile.Manufacturer = systemProperty("ro.product.manufacturer")
	profile.Model = systemProperty("ro.product.model")
	profile.Abi = systemProperty("ro.product.cpu.abi")
}

func systemProperty(name string) string {
	out, err := exec.Command("/system/bin/getprop", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
