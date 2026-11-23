package packages

// GuiPackages are the built-in packages that exist only in a cx built with
// the cxfx tag, `make build-gui`.
var GuiPackages = []string{"al", "gl", "glfw", "gltext"}

// GuiPackage reports whether name is one of GuiPackages.
func GuiPackage(name string) bool {
	for _, gui := range GuiPackages {
		if gui == name {
			return true
		}
	}
	return false
}
