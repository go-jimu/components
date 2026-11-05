package validation

import (
	"fmt"
	"strings"
)

type (
	// Specification is a reusable boolean predicate over a candidate. Combine
	// specifications with the New*Specification constructors.
	Specification interface {
		// String describes the rule, for example "(a) AND !(b)".
		fmt.Stringer
		// IsSatisfiedBy reports whether candidate meets the rule.
		IsSatisfiedBy(candidate interface{}) bool
	}

	// AndSpecification is built by NewAndSpecification.
	AndSpecification struct {
		left  Specification
		right Specification
	}

	// AndNotSpecification is built by NewAndNotSpecification.
	AndNotSpecification struct {
		left  Specification
		right Specification
	}

	// OrSpecification is built by NewOrSpecification.
	OrSpecification struct {
		left  Specification
		right Specification
	}

	// NotSpecification is built by NewNotSpecification.
	NotSpecification struct {
		spec Specification
	}

	// OrNotSpecification is built by NewOrNotSpecification.
	OrNotSpecification struct {
		left  Specification
		right Specification
	}

	// AllSpecification is built by NewAllSpecification.
	AllSpecification struct {
		specs []Specification
	}

	// AnySpecification is built by NewAnySpecification.
	AnySpecification struct {
		specs []Specification
	}
)

var (
	_ Specification = (*AndSpecification)(nil)
	_ Specification = (*OrSpecification)(nil)
	_ Specification = (*AndNotSpecification)(nil)
	_ Specification = (*OrNotSpecification)(nil)
	_ Specification = (*NotSpecification)(nil)
	_ Specification = (*AllSpecification)(nil)
	_ Specification = (*AnySpecification)(nil)
)

// NewAndSpecification returns a Specification satisfied when s1 and s2 both
// are. s2 is evaluated only when s1 is satisfied.
func NewAndSpecification(s1, s2 Specification) Specification {
	return &AndSpecification{s1, s2}
}

// IsSatisfiedBy reports whether candidate satisfies both specifications.
func (and *AndSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return and.left.IsSatisfiedBy(candidate) && and.right.IsSatisfiedBy(candidate)
}

func (and *AndSpecification) String() string {
	return fmt.Sprintf("(%s) AND (%s)", and.left.String(), and.right.String())
}

// NewOrSpecification returns a Specification satisfied when s1 or s2 is. s2
// is evaluated only when s1 is not satisfied.
func NewOrSpecification(s1, s2 Specification) Specification {
	return &OrSpecification{s1, s2}
}

// IsSatisfiedBy reports whether candidate satisfies either specification.
func (or *OrSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return or.left.IsSatisfiedBy(candidate) || or.right.IsSatisfiedBy(candidate)
}

func (or *OrSpecification) String() string {
	return fmt.Sprintf("(%s) OR (%s)", or.left.String(), or.right.String())
}

// NewAndNotSpecification returns a Specification satisfied when s1 is and s2
// is not. s2 is evaluated only when s1 is satisfied.
func NewAndNotSpecification(s1, s2 Specification) Specification {
	return &AndNotSpecification{s1, s2}
}

// IsSatisfiedBy reports whether candidate satisfies the left specification
// and not the right one.
func (an *AndNotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return an.left.IsSatisfiedBy(candidate) && (!an.right.IsSatisfiedBy(candidate))
}

func (an *AndNotSpecification) String() string {
	return fmt.Sprintf("(%s) AND !(%s)", an.left.String(), an.right.String())
}

// NewNotSpecification returns a Specification satisfied when spec is not.
func NewNotSpecification(spec Specification) Specification {
	return &NotSpecification{spec: spec}
}

// IsSatisfiedBy reports whether candidate fails the wrapped specification.
func (not *NotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return !not.spec.IsSatisfiedBy(candidate)
}

func (not *NotSpecification) String() string {
	return fmt.Sprintf("!(%s)", not.spec.String())
}

// NewOrNotSpecification returns a Specification satisfied when s1 is or s2 is
// not. s2 is evaluated only when s1 is not satisfied.
func NewOrNotSpecification(s1, s2 Specification) Specification {
	return &OrNotSpecification{s1, s2}
}

// IsSatisfiedBy reports whether candidate satisfies the left specification
// or fails the right one.
func (on *OrNotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return on.left.IsSatisfiedBy(candidate) || (!on.right.IsSatisfiedBy(candidate))
}

func (on *OrNotSpecification) String() string {
	return fmt.Sprintf("(%s) OR !(%s)", on.left.String(), on.right.String())
}

// NewAllSpecification returns a Specification satisfied when every one of
// specs is, evaluated in order up to the first failure. With no specs it is
// always satisfied.
func NewAllSpecification(specs ...Specification) Specification {
	return &AllSpecification{specs: specs}
}

// IsSatisfiedBy reports whether candidate satisfies every specification.
func (all *AllSpecification) IsSatisfiedBy(candidate interface{}) bool {
	for _, spec := range all.specs {
		if !spec.IsSatisfiedBy(candidate) {
			return false
		}
	}
	return true
}

func (all *AllSpecification) String() string {
	rules := make([]string, len(all.specs))
	for i := 0; i < len(all.specs); i++ {
		rules[i] = all.specs[i].String()
	}
	return fmt.Sprintf("ALL[%s]", strings.Join(rules, ", "))
}

// NewAnySpecification returns a Specification satisfied when at least one of
// specs is, evaluated in order up to the first match. With no specs it is
// never satisfied.
func NewAnySpecification(specs ...Specification) Specification {
	return &AnySpecification{specs: specs}
}

// IsSatisfiedBy reports whether candidate satisfies at least one
// specification.
func (anySpec *AnySpecification) IsSatisfiedBy(candidate interface{}) bool {
	for _, spec := range anySpec.specs {
		if spec.IsSatisfiedBy(candidate) {
			return true
		}
	}
	return false
}

func (anySpec *AnySpecification) String() string {
	rules := make([]string, len(anySpec.specs))
	for i := 0; i < len(anySpec.specs); i++ {
		rules[i] = anySpec.specs[i].String()
	}
	return fmt.Sprintf("ANY[%s]", strings.Join(rules, ", "))
}
