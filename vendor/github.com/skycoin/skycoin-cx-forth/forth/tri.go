package forth

// Three valued logic over False=-1, Null=0, True=1.
// Kleene strong logic: AND=min, OR=max, NOT=negate, IMPLIES = NOT a OR b.
// Lukasiewicz implication differs only at null->null, which is true.
// See Kleene, Introduction to Metamathematics (1952) section 64 and
// https://en.wikipedia.org/wiki/Three-valued_logic

// TriNot is Kleene negation.
func TriNot(a int64) int64 { return -a }

// TriAnd is Kleene conjunction.
func TriAnd(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// TriOr is Kleene disjunction.
func TriOr(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// TriImplies is Kleene material implication.
func TriImplies(a, b int64) int64 { return TriOr(-a, b) }

// TriLukImplies is Lukasiewicz implication, min(1, 1-a+b) in balanced form.
func TriLukImplies(a, b int64) int64 {
	v := 1 - a + b
	if v > 1 {
		return 1
	}
	return v
}

// TriXor is Kleene exclusive or. In balanced form it is -(a*b).
func TriXor(a, b int64) int64 { return -(a * b) }

// TriEqv is Kleene equivalence, a*b in balanced form.
func TriEqv(a, b int64) int64 { return a * b }
