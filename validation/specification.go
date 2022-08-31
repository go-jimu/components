package validation

import (
	"fmt"
	"strings"
)

type (
	// Specification define a interface
	Specification interface {
		// // String print the specification details
		fmt.Stringer
		// IsSatisfiedBy check if candidate matched the specification
		IsSatisfiedBy(candidate interface{}) bool
	}

	AndSpecification struct {
		left  Specification
		right Specification
	}

	AndNotSpecification struct {
		left  Specification
		right Specification
	}

	OrSpecification struct {
		left  Specification
		right Specification
	}

	NotSpecification struct {
		spec Specification
	}

	OrNotSpecification struct {
		left  Specification
		right Specification
	}

	AllSpecification struct {
		specs []Specification
	}

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
	_ Specification = (*AndSpecification)(nil)
)

func NewAndSpecification(s1, s2 Specification) Specification {
	return &AndSpecification{s1, s2}
}

func (and *AndSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return and.left.IsSatisfiedBy(candidate) && and.right.IsSatisfiedBy(candidate)
}

func (and *AndSpecification) String() string {
	return fmt.Sprintf("(%s) AND (%s)", and.left.String(), and.right.String())
}

func NewOrSpecification(s1, s2 Specification) Specification {
	return &OrSpecification{s1, s2}
}

func (or *OrSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return or.left.IsSatisfiedBy(candidate) || or.right.IsSatisfiedBy(candidate)
}

func (or *OrSpecification) String() string {
	return fmt.Sprintf("(%s) OR (%s)", or.left.String(), or.right.String())
}

func NewAndNotSpecification(s1, s2 Specification) Specification {
	return &AndNotSpecification{s1, s2}
}

func (an *AndNotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return an.left.IsSatisfiedBy(candidate) && (!an.right.IsSatisfiedBy(candidate))
}

func (an *AndNotSpecification) String() string {
	return fmt.Sprintf("(%s) AND !(%s)", an.left.String(), an.right.String())
}

func NewNotSpecification(spec Specification) Specification {
	return &NotSpecification{spec: spec}
}

func (not *NotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return !not.spec.IsSatisfiedBy(candidate)
}

func (not *NotSpecification) String() string {
	return fmt.Sprintf("!(%s)", not.spec.String())
}

func NewOrNotSpecification(s1, s2 Specification) Specification {
	return &OrNotSpecification{s1, s2}
}

func (on *OrNotSpecification) IsSatisfiedBy(candidate interface{}) bool {
	return on.left.IsSatisfiedBy(candidate) || (!on.right.IsSatisfiedBy(candidate))
}

func (on *OrNotSpecification) String() string {
	return fmt.Sprintf("(%s) OR !(%s)", on.left.String(), on.right.String())
}

func NewAllSpecification(specs ...Specification) Specification {
	return &AllSpecification{specs: specs}
}

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

func NewAnySpecification(specs ...Specification) Specification {
	return &AnySpecification{specs: specs}
}

func (any *AnySpecification) IsSatisfiedBy(candidate interface{}) bool {
	for _, spec := range any.specs {
		if spec.IsSatisfiedBy(candidate) {
			return true
		}
	}
	return false
}

func (any *AnySpecification) String() string {
	rules := make([]string, len(any.specs))
	for i := 0; i < len(any.specs); i++ {
		rules[i] = any.specs[i].String()
	}
	return fmt.Sprintf("ANY[%s]", strings.Join(rules, ", "))
}
