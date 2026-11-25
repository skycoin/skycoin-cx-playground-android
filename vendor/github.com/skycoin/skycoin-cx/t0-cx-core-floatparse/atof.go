// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Ported from internal/strconv/atof.go, atoi.go, deps.go and ftoa.go of Go 1.27.3.
// The input is a byte slice instead of a string, errors are a
// FloatParseResult instead of an error value, and the inf, infinity and nan
// prefixes are byte arrays, so no string operation is left (#412, D92).

package floatparse

import "math"

const (
	float32MantBits = 23
	float32ExpBits  = 8
	float32Bias     = -127

	float64MantBits = 52
	float64ExpBits  = 11
	float64Bias     = -1023
)

func inf(sign int) float64 {
	return math.Inf(sign)
}

func nan() float64 { return math.Float64frombits(0x7FF8000000000001) }

// lower(c) is a lower-case letter if and only if
// c is either that lower-case letter or the equivalent upper-case letter.
// Instead of writing c == 'x' || c == 'X' one can write lower(c) == 'x'.
// Note that lower of non-letters can produce other non-letters.
func lower(c byte) byte {
	return c | ('x' - 'X')
}

var (
	prefixInfinity = [...]byte{'i', 'n', 'f', 'i', 'n', 'i', 't', 'y'}
	prefixNaN      = [...]byte{'n', 'a', 'n'}
)

// optimize false forces the slow path, for tests.
var optimize = true

type floatInfo struct {
	mantbits uint
	expbits  uint
	bias     int
}

var (
	float32info = floatInfo{float32MantBits, float32ExpBits, float32Bias}
	float64info = floatInfo{float64MantBits, float64ExpBits, float64Bias}
)

// commonPrefixLenIgnoreCase returns the length of the common
// prefix of s and prefix, with the character case of s ignored.
// The prefix argument must be all lower-case.
func commonPrefixLenIgnoreCase(s, prefix []byte) int {
	n := min(len(prefix), len(s))
	for i := 0; i < n; i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return i
		}
	}
	return n
}

// special returns the floating-point value for the special,
// possibly signed floating-point representations inf, infinity,
// and NaN. The result is ok if a prefix of s contains one
// of these representations and n is the length of that prefix.
// The character case is ignored.
func special(s []byte) (f float64, n int, ok bool) {
	if len(s) == 0 {
		return 0, 0, false
	}
	sign := 1
	nsign := 0
	switch s[0] {
	case '+', '-':
		if s[0] == '-' {
			sign = -1
		}
		nsign = 1
		s = s[1:]
		fallthrough
	case 'i', 'I':
		n := commonPrefixLenIgnoreCase(s, prefixInfinity[:])
		// Anything longer than "inf" is ok, but if we
		// don't have "infinity", only consume "inf".
		if 3 < n && n < 8 {
			n = 3
		}
		if n == 3 || n == 8 {
			return inf(sign), nsign + n, true
		}
	case 'n', 'N':
		if commonPrefixLenIgnoreCase(s, prefixNaN[:]) == 3 {
			return nan(), 3, true
		}
	}
	return 0, 0, false
}

func (b *decimal) set(s []byte) (ok bool) {
	i := 0
	b.neg = false
	b.trunc = false

	// optional sign
	if i >= len(s) {
		return
	}
	switch s[i] {
	case '+':
		i++
	case '-':
		i++
		b.neg = true
	}

	// digits
	sawdot := false
	sawdigits := false
	for ; i < len(s); i++ {
		switch {
		case s[i] == '_':
			// readFloat already checked underscores
			continue
		case s[i] == '.':
			if sawdot {
				return
			}
			sawdot = true
			b.dp = b.nd
			continue

		case '0' <= s[i] && s[i] <= '9':
			sawdigits = true
			if s[i] == '0' && b.nd == 0 { // ignore leading zeros
				b.dp--
				continue
			}
			if b.nd < len(b.d) {
				b.d[b.nd] = s[i]
				b.nd++
			} else if s[i] != '0' {
				b.trunc = true
			}
			continue
		}
		break
	}
	if !sawdigits {
		return
	}
	if !sawdot {
		b.dp = b.nd
	}

	// optional exponent moves decimal point.
	// if we read a very large, very long number,
	// just be sure to move the decimal point by
	// a lot (say, 100000).  it doesn't matter if it's
	// not the exact number.
	if i < len(s) && lower(s[i]) == 'e' {
		i++
		if i >= len(s) {
			return
		}
		esign := 1
		switch s[i] {
		case '+':
			i++
		case '-':
			i++
			esign = -1
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return
		}
		e := 0
		for ; i < len(s) && ('0' <= s[i] && s[i] <= '9' || s[i] == '_'); i++ {
			if s[i] == '_' {
				// readFloat already checked underscores
				continue
			}
			if e < 10000 {
				e = e*10 + int(s[i]) - '0'
			}
		}
		b.dp += e * esign
	}

	if i != len(s) {
		return
	}

	ok = true
	return
}

// readFloat reads a decimal or hexadecimal mantissa and exponent from a float
// string representation in s; the number may be followed by other characters.
// readFloat reports the number of bytes consumed (i), and whether the number
// is valid (ok).
func readFloat(s []byte) (mantissa uint64, exp int, neg, trunc, hex bool, i int, ok bool) {
	underscores := false

	// optional sign
	if i >= len(s) {
		return
	}
	switch s[i] {
	case '+':
		i++
	case '-':
		i++
		neg = true
	}

	// digits
	base := uint64(10)
	maxMantDigits := 19 // 10^19 fits in uint64
	expChar := byte('e')
	if i+2 < len(s) && s[i] == '0' && lower(s[i+1]) == 'x' {
		base = 16
		maxMantDigits = 16 // 16^16 fits in uint64
		i += 2
		expChar = 'p'
		hex = true
	}
	sawdot := false
	sawdigits := false
	nd := 0
	ndMant := 0
	dp := 0
loop:
	for ; i < len(s); i++ {
		switch c := s[i]; true {
		case c == '_':
			underscores = true
			continue

		case c == '.':
			if sawdot {
				break loop
			}
			sawdot = true
			dp = nd
			continue

		case '0' <= c && c <= '9':
			sawdigits = true
			if c == '0' && nd == 0 { // ignore leading zeros
				dp--
				continue
			}
			nd++
			if ndMant < maxMantDigits {
				mantissa *= base
				mantissa += uint64(c - '0')
				ndMant++
			} else if c != '0' {
				trunc = true
			}
			continue

		case base == 16 && 'a' <= lower(c) && lower(c) <= 'f':
			sawdigits = true
			nd++
			if ndMant < maxMantDigits {
				mantissa *= 16
				mantissa += uint64(lower(c) - 'a' + 10)
				ndMant++
			} else {
				trunc = true
			}
			continue
		}
		break
	}
	if !sawdigits {
		return
	}
	if !sawdot {
		dp = nd
	}

	if base == 16 {
		dp *= 4
		ndMant *= 4
	}

	// optional exponent moves decimal point.
	// if we read a very large, very long number,
	// just be sure to move the decimal point by
	// a lot (say, 100000).  it doesn't matter if it's
	// not the exact number.
	if i < len(s) && lower(s[i]) == expChar {
		i++
		if i >= len(s) {
			return
		}
		esign := 1
		switch s[i] {
		case '+':
			i++
		case '-':
			i++
			esign = -1
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return
		}
		e := 0
		for ; i < len(s) && ('0' <= s[i] && s[i] <= '9' || s[i] == '_'); i++ {
			if s[i] == '_' {
				underscores = true
				continue
			}
			if e < 10000 {
				e = e*10 + int(s[i]) - '0'
			}
		}
		dp += e * esign
	} else if base == 16 {
		// Must have exponent.
		return
	}

	if mantissa != 0 {
		exp = dp - ndMant
	}

	if underscores && !underscoreOK(s[:i]) {
		return
	}

	ok = true
	return
}

// decimal power of ten to binary power of two.
var powtab = []int{1, 3, 6, 9, 13, 16, 19, 23, 26}

func (d *decimal) floatBits(flt *floatInfo) (b uint64, overflow bool) {
	var exp int
	var mant uint64

	// Zero is always a special case.
	if d.nd == 0 {
		mant = 0
		exp = flt.bias
		goto out
	}

	// Obvious overflow/underflow.
	// These bounds are for 64-bit floats.
	// Will have to change if we want to support 80-bit floats in the future.
	if d.dp > 310 {
		goto overflow
	}
	if d.dp < -330 {
		// zero
		mant = 0
		exp = flt.bias
		goto out
	}

	// Scale by powers of two until in range [0.5, 1.0)
	exp = 0
	for d.dp > 0 {
		var n int
		if d.dp >= len(powtab) {
			n = 27
		} else {
			n = powtab[d.dp]
		}
		d.Shift(-n)
		exp += n
	}
	for d.dp < 0 || d.dp == 0 && d.d[0] < '5' {
		var n int
		if -d.dp >= len(powtab) {
			n = 27
		} else {
			n = powtab[-d.dp]
		}
		d.Shift(n)
		exp -= n
	}

	// Our range is [0.5,1) but floating point range is [1,2).
	exp--

	// Minimum representable exponent is flt.bias+1.
	// If the exponent is smaller, move it up and
	// adjust d accordingly.
	if exp < flt.bias+1 {
		n := flt.bias + 1 - exp
		d.Shift(-n)
		exp += n
	}

	if exp-flt.bias >= 1<<flt.expbits-1 {
		goto overflow
	}

	// Extract 1+flt.mantbits bits.
	d.Shift(int(1 + flt.mantbits))
	mant = d.RoundedInteger()

	// Rounding might have added a bit; shift down.
	if mant == 2<<flt.mantbits {
		mant >>= 1
		exp++
		if exp-flt.bias >= 1<<flt.expbits-1 {
			goto overflow
		}
	}

	// Denormalized?
	if mant&(1<<flt.mantbits) == 0 {
		exp = flt.bias
	}
	goto out

overflow:
	// ±Inf
	mant = 0
	exp = 1<<flt.expbits - 1 + flt.bias
	overflow = true

out:
	// Assemble bits.
	bits := mant & (uint64(1)<<flt.mantbits - 1)
	bits |= uint64((exp-flt.bias)&(1<<flt.expbits-1)) << flt.mantbits
	if d.neg {
		bits |= 1 << flt.mantbits << flt.expbits
	}
	return bits, overflow
}

// Exact powers of 10.
var float64pow10 = []float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
	1e20, 1e21, 1e22,
}
var float32pow10 = []float32{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10}

// If possible to convert decimal representation to 64-bit float f exactly,
// entirely in floating-point math, do so, avoiding the expense of decimalToFloatBits.
// Three common cases:
//
//	value is exact integer
//	value is exact integer * exact power of ten
//	value is exact integer / exact power of ten
//
// These all produce potentially inexact but correctly rounded answers.
func atof64exact(mantissa uint64, exp int, neg bool) (f float64, ok bool) {
	if mantissa>>float64info.mantbits != 0 {
		return
	}
	f = float64(mantissa)
	if neg {
		f = -f
	}
	switch {
	case exp == 0:
		// an integer.
		return f, true
	// Exact integers are <= 10^15.
	// Exact powers of ten are <= 10^22.
	case exp > 0 && exp <= 15+22: // int * 10^k
		// If exponent is big but number of digits is not,
		// can move a few zeros into the integer part.
		if exp > 22 {
			f *= float64pow10[exp-22]
			exp = 22
		}
		if f > 1e15 || f < -1e15 {
			// the exponent was really too large.
			return
		}
		return f * float64pow10[exp], true
	case exp < 0 && exp >= -22: // int / 10^k
		return f / float64pow10[-exp], true
	}
	return
}

// If possible to compute mantissa*10^exp to 32-bit float f exactly,
// entirely in floating-point math, do so, avoiding the machinery above.
func atof32exact(mantissa uint64, exp int, neg bool) (f float32, ok bool) {
	if mantissa>>float32MantBits != 0 {
		return
	}
	f = float32(mantissa)
	if neg {
		f = -f
	}
	switch {
	case exp == 0:
		return f, true
	// Exact integers are <= 10^7.
	// Exact powers of ten are <= 10^10.
	case exp > 0 && exp <= 7+10: // int * 10^k
		// If exponent is big but number of digits is not,
		// can move a few zeros into the integer part.
		if exp > 10 {
			f *= float32pow10[exp-10]
			exp = 10
		}
		if f > 1e7 || f < -1e7 {
			// the exponent was really too large.
			return
		}
		return f * float32pow10[exp], true
	case exp < 0 && exp >= -10: // int / 10^k
		return f / float32pow10[-exp], true
	}
	return
}

// atofHex converts the hex floating-point string s
// to a rounded float32 or float64 value (depending on flt==&float32info or flt==&float64info)
// and returns it as a float64.
// The string s has already been parsed into a mantissa, exponent, and sign (neg==true for negative).
// If trunc is true, trailing non-zero bits have been omitted from the mantissa.
func atofHex(s []byte, flt *floatInfo, mantissa uint64, exp int, neg, trunc bool) (float64, FloatParseResult) {
	maxExp := 1<<flt.expbits + flt.bias - 2
	minExp := flt.bias + 1
	exp += int(flt.mantbits) // mantissa now implicitly divided by 2^mantbits.

	// Shift mantissa and exponent to bring representation into float range.
	// Eventually we want a mantissa with a leading 1-bit followed by mantbits other bits.
	// For rounding, we need two more, where the bottom bit represents
	// whether that bit or any later bit was non-zero.
	// (If the mantissa has already lost non-zero bits, trunc is true,
	// and we OR in a 1 below after shifting left appropriately.)
	for mantissa != 0 && mantissa>>(flt.mantbits+2) == 0 {
		mantissa <<= 1
		exp--
	}
	if trunc {
		mantissa |= 1
	}
	for mantissa>>(1+flt.mantbits+2) != 0 {
		mantissa = mantissa>>1 | mantissa&1
		exp++
	}

	// If exponent is too negative,
	// denormalize in hopes of making it representable.
	// (The -2 is for the rounding bits.)
	for mantissa > 1 && exp < minExp-2 {
		mantissa = mantissa>>1 | mantissa&1
		exp++
	}

	// Round using two bottom bits.
	round := mantissa & 3
	mantissa >>= 2
	round |= mantissa & 1 // round to even (round up if mantissa is odd)
	exp += 2
	if round == 3 {
		mantissa++
		if mantissa == 1<<(1+flt.mantbits) {
			mantissa >>= 1
			exp++
		}
	}

	if mantissa>>flt.mantbits == 0 { // Denormal or zero.
		exp = flt.bias
	}
	var err FloatParseResult
	if exp > maxExp { // infinity and range error
		mantissa = 1 << flt.mantbits
		exp = maxExp + 1
		err = FloatParseResultRange
	}

	bits := mantissa & (1<<flt.mantbits - 1)
	bits |= uint64((exp-flt.bias)&(1<<flt.expbits-1)) << flt.mantbits
	if neg {
		bits |= 1 << flt.mantbits << flt.expbits
	}
	if flt == &float32info {
		return float64(math.Float32frombits(uint32(bits))), err
	}
	return math.Float64frombits(bits), err
}

func atof32(s []byte) (f float32, n int, err FloatParseResult) {
	if val, n, ok := special(s); ok {
		return float32(val), n, FloatParseResultOK
	}

	d, p, neg, trunc, hex, n, ok := readFloat(s)
	if !ok {
		return 0, n, FloatParseResultSyntax
	}

	if hex {
		f, err := atofHex(s[:n], &float32info, d, p, neg, trunc)
		return float32(f), n, err
	}

	if optimize {
		sign := bool2[uint32](neg) << 31
		if d == 0 {
			return math.Float32frombits(sign | 0), n, FloatParseResultOK
		}
		if p > 40 { // overflow to ±Inf
			return math.Float32frombits(sign | 0xff<<23), n, FloatParseResultRange
		}
		if p < -70 { // underflow to ±0
			return math.Float32frombits(sign | 0), n, FloatParseResultOK
		}
		if !trunc {
			// Exact rounding with single multiplication or division.
			if f, ok := atof32exact(d, p, neg); ok {
				return f, n, FloatParseResultOK
			}
		}
		// Use fast unrounded scaling.
		// The only possible err is FloatParseResultRange, when the result overflows to ±Inf.
		f, err := parseFloat32(d, p, sign)
		if !trunc {
			return f, n, err
		}
		// If additional digits were truncated from d
		// but d+1 converts to the same value,
		// then the additional digits don't matter.
		f1, _ := parseFloat32(d+1, p, sign)
		if f == f1 {
			return f, n, err
		}
	}

	// Slow fallback.
	var dec decimal
	if !dec.set(s[:n]) {
		return 0, n, FloatParseResultSyntax
	}
	b, ovf := dec.floatBits(&float32info)
	f = math.Float32frombits(uint32(b))
	if ovf {
		err = FloatParseResultRange
	}
	return f, n, err
}

func atof64(s []byte) (f float64, n int, err FloatParseResult) {
	if val, n, ok := special(s); ok {
		return val, n, FloatParseResultOK
	}

	d, p, neg, trunc, hex, n, ok := readFloat(s)
	if !ok {
		return 0, n, FloatParseResultSyntax
	}
	if hex {
		f, err := atofHex(s[:n], &float64info, d, p, neg, trunc)
		return f, n, err
	}
	if optimize {
		sign := bool2[uint64](neg) << 63
		if d == 0 {
			return math.Float64frombits(sign | 0), n, FloatParseResultOK
		}
		if p > 310 { // overflow to ±Inf
			return math.Float64frombits(sign | 0x7ff<<52), n, FloatParseResultRange
		}
		if p < -345 { // underflow to ±0
			return math.Float64frombits(sign | 0), n, FloatParseResultOK
		}
		if !trunc {
			// Exact rounding with single multiplication or division.
			if f, ok := atof64exact(d, p, neg); ok {
				return f, n, FloatParseResultOK
			}
		}
		// Use fast unrounded scaling.
		// The only possible err is FloatParseResultRange, when the result overflows to ±Inf.
		f, err := parseFloat64(d, p, sign)
		if !trunc {
			return f, n, err
		}
		// If additional digits were truncated from d
		// but d+1 converts to the same value,
		// then the additional digits don't matter.
		f1, _ := parseFloat64(d+1, p, sign)
		if f == f1 {
			return f, n, err
		}
	}

	// Slow fallback.
	var dec decimal
	if !dec.set(s[:n]) {
		return 0, n, FloatParseResultSyntax
	}
	b, ovf := dec.floatBits(&float64info)
	f = math.Float64frombits(b)
	if ovf {
		err = FloatParseResultRange
	}
	return f, n, err
}

// FloatParseResult says whether FloatParseBytes parsed its input.
type FloatParseResult uint8

const (
	// FloatParseResultOK is a well formed number in range.
	FloatParseResultOK FloatParseResult = iota
	// FloatParseResultRange is a well formed number more than 1/2 ULP past
	// the largest float of the size, strconv.ErrRange. The value is ±Inf.
	FloatParseResultRange
	// FloatParseResultSyntax is malformed input, strconv.ErrSyntax. The
	// value is 0.
	FloatParseResultSyntax
)

// FloatParseBytes is strconv.ParseFloat(string(text), bitSize) without the
// string. It returns the same bits, and the result names the strconv error.
// Like strconv, a bitSize other than 32 parses a float64.
func FloatParseBytes(text []byte, bitSize int) (float64, FloatParseResult) {
	f, n, result := parseFloatPrefix(text, bitSize)
	if n != len(text) {
		return 0, FloatParseResultSyntax
	}
	return f, result
}

func parseFloatPrefix(s []byte, bitSize int) (float64, int, FloatParseResult) {
	if bitSize == 32 {
		f, n, err := atof32(s)
		return float64(f), n, err
	}
	return atof64(s)
}

// underscoreOK reports whether the underscores in s are allowed.
// Checking them in this one function lets all the parsers skip over them simply.
// Underscore must appear only between digits or between a base prefix and a digit.
func underscoreOK(s []byte) bool {
	// saw tracks the last character (class) we saw:
	// ^ for beginning of number,
	// 0 for a digit or base prefix,
	// _ for an underscore,
	// ! for none of the above.
	saw := '^'
	i := 0

	// Optional sign.
	if len(s) >= 1 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}

	// Optional base prefix.
	hex := false
	if len(s) >= 2 && s[0] == '0' && (lower(s[1]) == 'b' || lower(s[1]) == 'o' || lower(s[1]) == 'x') {
		i = 2
		saw = '0' // base prefix counts as a digit for "underscore as digit separator"
		hex = lower(s[1]) == 'x'
	}

	// Number proper.
	for ; i < len(s); i++ {
		// Digits are always okay.
		if '0' <= s[i] && s[i] <= '9' || hex && 'a' <= lower(s[i]) && lower(s[i]) <= 'f' {
			saw = '0'
			continue
		}
		// Underscore must follow digit.
		if s[i] == '_' {
			if saw != '0' {
				return false
			}
			saw = '_'
			continue
		}
		// Underscore must also be followed by digit.
		if saw == '_' {
			return false
		}
		// Saw non-digit, non-underscore.
		saw = '!'
	}
	return saw != '_'
}
