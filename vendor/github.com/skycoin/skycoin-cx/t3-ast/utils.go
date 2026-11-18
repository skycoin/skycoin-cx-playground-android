package ast

import "github.com/skycoin/skycoin-cx/t0-types"

func TotalLength(lengths []types.Pointer) types.Pointer {
	total := types.Pointer(1)
	for _, i := range lengths {
		total *= i
	}

	return total
}
