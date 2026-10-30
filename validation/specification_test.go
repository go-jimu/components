package validation

import (
	"fmt"
	"testing"
)

type testSpecV1 struct{}
type testSpecV2 struct{}

func (ts *testSpecV1) IsSatisfiedBy(candidate interface{}) bool {
	v, ok := candidate.(int)
	if ok {
		return v >= 10
	}
	return false
}

func (ts *testSpecV1) String() string {
	return "value >= 10"
}

func (ts2 *testSpecV2) IsSatisfiedBy(candidate interface{}) bool {
	v, ok := candidate.(int)
	if ok {
		return v < 80
	}
	return false
}

func (ts2 *testSpecV2) String() string {
	return "value < 80"
}

func TestSpec(t *testing.T) {
	var s1 = new(testSpecV1)
	var s2 = new(testSpecV1)
	var s3 = new(testSpecV2)

	a1 := NewAndSpecification(s1, s2)
	fmt.Println(a1)
	if !a1.IsSatisfiedBy(20) {
		t.FailNow()
	}
	if a1.IsSatisfiedBy(5) {
		t.FailNow()
	}

	a2 := NewAndSpecification(a1, s3)
	fmt.Println(a2)
	if !a2.IsSatisfiedBy(50) {
		t.FailNow()
	}

	a3 := NewAndNotSpecification(a1, s3)
	fmt.Println(a3)
	if !a3.IsSatisfiedBy(90) {
		t.FailNow()
	}
	if a3.IsSatisfiedBy(70) {
		t.FailNow()
	}

	a4 := NewAnySpecification(s1, a3)
	fmt.Println(a4)
	if !a4.IsSatisfiedBy(40) {
		t.FailNow()
	}
}

// Intent: every combinator evaluates its operands with boolean logic and
// describes itself from its operands.
func TestSpecificationCombinators(t *testing.T) {
	ge10 := new(testSpecV1) // value >= 10
	lt80 := new(testSpecV2) // value < 80
	inputs := []int{5, 50, 90}

	tests := []struct {
		name string
		spec Specification
		want []bool // for each of inputs
		str  string
	}{
		{"and", NewAndSpecification(ge10, lt80), []bool{false, true, false}, "(value >= 10) AND (value < 80)"},
		{"or", NewOrSpecification(ge10, lt80), []bool{true, true, true}, "(value >= 10) OR (value < 80)"},
		{"and not", NewAndNotSpecification(ge10, lt80), []bool{false, false, true}, "(value >= 10) AND !(value < 80)"},
		{"or not", NewOrNotSpecification(lt80, ge10), []bool{true, true, false}, "(value < 80) OR !(value >= 10)"},
		{"not", NewNotSpecification(ge10), []bool{true, false, false}, "!(value >= 10)"},
		{"all", NewAllSpecification(ge10, lt80), []bool{false, true, false}, "ALL[value >= 10, value < 80]"},
		{"any", NewAnySpecification(ge10, lt80), []bool{true, true, true}, "ANY[value >= 10, value < 80]"},
		{"empty all", NewAllSpecification(), []bool{true, true, true}, "ALL[]"},
		{"empty any", NewAnySpecification(), []bool{false, false, false}, "ANY[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, in := range inputs {
				if got := tt.spec.IsSatisfiedBy(in); got != tt.want[i] {
					t.Fatalf("IsSatisfiedBy(%d) = %v, want %v", in, got, tt.want[i])
				}
			}
			if got := tt.spec.String(); got != tt.str {
				t.Fatalf("String() = %q, want %q", got, tt.str)
			}
		})
	}
}
