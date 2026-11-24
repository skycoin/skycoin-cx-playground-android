package types

//"fmt"

func Write_str_data(memory []byte, offset Pointer, value string) {
	bytes := []byte(value)
	size := Compute_obj_size(bytes)
	Write_obj_data(memory, offset, size, bytes)
}

// VIOLATION STRHOT-009: B, reads a str for the affordance opcodes, #412.
// VIOLATION VMALLOC-003: a Go string copy of a str, for the boundary opcodes only, #407.
func Read_str_data(memory []byte, offset Pointer) string {
	str := Read_obj_data(memory, offset)
	return string(str)
}

// VIOLATION STRHOT-011: B, only the boundary opcodes use Go strings, #412.
// VIOLATION VMALLOC-003: a Go string copy of a str, for the boundary opcodes only, #407.
func Read_str(memory []byte, offset Pointer) string {
	str := string(Read_obj(memory, offset))
	return str
}

// Read_str_bytes returns the bytes of the str whose pointer is at offset, a
// view into memory without a copy. The empty str is nil.
func Read_str_bytes(memory []byte, offset Pointer) []byte {
	return Read_obj(memory, offset)
}

func Read_str_size(memory []byte, offset Pointer) Pointer {
	heapOffset := Read_ptr(memory, offset)
	if heapOffset > 0 && heapOffset.IsValid() {
		return Read_obj_size(memory, heapOffset) - OBJECT_HEADER_SIZE
	}
	return 0
}
