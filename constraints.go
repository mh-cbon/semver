package semver

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Constraints is one or more constraint that a semantic version can be
// checked against.
type Constraints struct {
	constraints []constraintGroup
}

// NewConstraint returns a Constraints instance that a Version instance can
// be checked against. If there is a parse error it will be returned.
func NewConstraint(c string) (*Constraints, error) {

	// Rewrite - ranges into a comparison operation.
	c = rewriteRange(c)

	ors := strings.Split(c, "||")
	or := make([]constraintGroup, len(ors))
	for k, v := range ors {
		cs := strings.Split(v, ",")
		result := make(constraintGroup, len(cs))
		for i, s := range cs {
			pc, err := parseConstraint(s)
			if err != nil {
				return nil, err
			}

			result[i] = pc
		}
		or[k] = result
	}

	o := &Constraints{constraints: or}
	return o, nil
}

func NewConstraintNu(c string) (Constraint, error) {
	// Rewrite - ranges into a comparison operation.
	c = rewriteRange(c)

	ors := strings.Split(c, "||")
	or := make([]Constraint, len(ors))
	for k, v := range ors {
		cs := strings.Split(v, ",")
		result := make([]Constraint, len(cs))
		for i, s := range cs {
			pc, err := parseConstraintNu(s)
			if err != nil {
				return nil, err
			}

			result[i] = pc
		}
		or[k] = Intersection(result...)
	}

	return Union(or...), nil
}

// Check tests if a version satisfies the constraints.
func (cs Constraints) Check(v *Version) bool {
	// loop over the ORs and check the inner ANDs
	for _, o := range cs.constraints {
		joy := true
		for _, c := range o {
			if !c.check(v) {
				joy = false
				break
			}
		}

		if joy {
			return true
		}
	}

	return false
}

// Validate checks if a version satisfies a constraint. If not a slice of
// reasons for the failure are returned in addition to a bool.
func (cs Constraints) Validate(v *Version) (bool, []error) {
	// loop over the ORs and check the inner ANDs
	var e []error
	for _, o := range cs.constraints {
		joy := true
		for _, c := range o {
			if !c.check(v) {
				em := fmt.Errorf(c.msg, v, c.orig)
				e = append(e, em)
				joy = false
			}
		}

		if joy {
			return true, []error{}
		}
	}

	return false, e
}

/*
func (cs Constraints) Intersect(other ...*Constraints) *Constraints {
	// TODO not a pointer receiver...just overwrite cs?
	rc := cs

	// Extract the receiver's range

	for _, o := range other {
		for _, grp := range o.constraints {
			if len(grp) == 0 {
				// not sure how this would happen, but make sure we skip it
				continue
			}
			c := grp.asConstraint()
			if c == nil {
				// no match at all, wtf, panic
				panic("unreachable?")
			}

			switch r := c.(type) {
			case none:
				// Arriving at 'None' at any point guarantees our final answer
				// will also be 'None'
				// TODO ugh clean up how this is done
				return &Constraints{}
			case *Version:
				// TODO ...bleh
				return &Constraints{
					constraints: []constraintGroup{
						&constraint{
							function:   constraintTildeOrEqual,
							msg:        constraintMsg["="],
							operand:    "=",
							con:        r,
							minorDirty: false, // OK?
							dirty:      false, // OK?
						},
					},
				}
			}

			// no min or max; the range must only have exact matches/negations
			if rng.min != nil || rng.max != nil {
			}
		}
	}

	return &rc
}
*/
type constraintGroup []*constraint

/*
func (cg constraintGroup) asConstraint() Constraint {
	if len(cg) == 0 {
		return nil
	}

	// TODO initialize rangeConstraint with appropriate min (zero) and max
	// (Inf?) versions
	rc := &rangeConstraint{}

	// TODO because constraint building itself doesn't dedupe these, we always have to
	// walk the whole list
	for _, c := range cg {
		switch c.predicate {
		case "^", "~", "~>", ">", ">=", "=>":
			if rc.min == nil {
				rc.min = c
			} else if c.predicate == ">" && rc.min.predicate != ">" {
				// Different handling if current is gte, but new is just gt
				if rc.min.con.LessThan(c.con) {
					rc.min = c
				}
			} else if c.con.LessThan(rc.min.con) {
				rc.min = c
			}
		case "<", "<=", "=<":
			if rc.max == nil {
				rc.max = c
			} else if c.predicate == "<" && rc.max.predicate != "<" {
				if rc.max.con.GreaterThan(c.con) {
					rc.max = c
				}
			} else if c.con.GreaterThan(rc.max.con) {
				rc.max = c
			}
		case "!=":
			// drop excluded versions onto the appropriate list
			rc.excl = append(rc.excl, c)
		case "", "=":
			// An exact match constraint has greater specificity, and zero
			// flexibility; this group can't be a range
			// TODO possible to have *more* than one exact version? shouldn't
			// be, but...
			return c.con
		}
	}

	return rc
}
*/

var constraintOps map[string]cfunc
var constraintMsg map[string]string
var constraintRegex *regexp.Regexp

func init() {
	constraintOps = map[string]cfunc{
		"":   constraintTildeOrEqual,
		"=":  constraintTildeOrEqual,
		"!=": constraintNotEqual,
		">":  constraintGreaterThan,
		"<":  constraintLessThan,
		">=": constraintGreaterThanEqual,
		"=>": constraintGreaterThanEqual,
		"<=": constraintLessThanEqual,
		"=<": constraintLessThanEqual,
		"~":  constraintTilde,
		"~>": constraintTilde,
		"^":  constraintCaret,
	}

	constraintMsg = map[string]string{
		"":   "%s is not equal to %s",
		"=":  "%s is not equal to %s",
		"!=": "%s is equal to %s",
		">":  "%s is less than or equal to %s",
		"<":  "%s is greater than or equal to %s",
		">=": "%s is less than %s",
		"=>": "%s is less than %s",
		"<=": "%s is greater than %s",
		"=<": "%s is greater than %s",
		"~":  "%s does not have same major and minor version as %s",
		"~>": "%s does not have same major and minor version as %s",
		"^":  "%s does not have same major version as %s",
	}

	ops := make([]string, 0, len(constraintOps))
	for k := range constraintOps {
		ops = append(ops, regexp.QuoteMeta(k))
	}

	constraintRegex = regexp.MustCompile(fmt.Sprintf(
		`^\s*(%s)\s*(%s)\s*$`,
		strings.Join(ops, "|"),
		cvRegex))

	constraintRangeRegex = regexp.MustCompile(fmt.Sprintf(
		`\s*(%s)\s*-\s*(%s)\s*`,
		cvRegex, cvRegex))
}

// An individual constraint
type constraint struct {
	// The callback function for the restraint. It performs the logic for
	// the constraint.
	function cfunc

	msg string

	// The version used in the constraint check. For example, if a constraint
	// is '<= 2.0.0' the con a version instance representing 2.0.0.
	con *Version

	// The operator predicate applied to this constaint
	predicate string

	// The original parsed version (e.g., 4.x from != 4.x)
	orig string

	// When an x is used as part of the version (e.g., 1.x)
	minorDirty bool
	dirty      bool
}

// Check if a version meets the constraint
func (c *constraint) check(v *Version) bool {
	return c.function(v, c)
}

type cfunc func(v *Version, c *constraint) bool

func parseConstraint(c string) (*constraint, error) {
	m := constraintRegex.FindStringSubmatch(c)
	if m == nil {
		return nil, fmt.Errorf("improper constraint: %s", c)
	}

	ver := m[2]
	orig := ver
	minorDirty := false
	dirty := false
	if isX(m[3]) {
		ver = "0.0.0"
		dirty = true
	} else if isX(strings.TrimPrefix(m[4], ".")) {
		minorDirty = true
		dirty = true
		ver = fmt.Sprintf("%s.0.0%s", m[3], m[6])
	} else if isX(strings.TrimPrefix(m[5], ".")) {
		dirty = true
		ver = fmt.Sprintf("%s%s.0%s", m[3], m[4], m[6])
	}

	con, err := NewVersion(ver)
	if err != nil {

		// The constraintRegex should catch any regex parsing errors. So,
		// we should never get here.
		return nil, errors.New("constraint Parser Error")
	}

	cs := &constraint{
		function:   constraintOps[m[1]],
		msg:        constraintMsg[m[1]],
		predicate:  m[1],
		con:        con,
		orig:       orig,
		minorDirty: minorDirty,
		dirty:      dirty,
	}
	return cs, nil
}

func parseConstraintNu(c string) (Constraint, error) {
	m := constraintRegex.FindStringSubmatch(c)
	if m == nil {
		return nil, fmt.Errorf("Malformed constraint: %s", c)
	}

	// Handle the full wildcard case first - easy!
	if isX(m[3]) {
		return any{}, nil
	}

	ver := m[2]
	var wildPatch, wildMinor bool
	if isX(strings.TrimPrefix(m[4], ".")) {
		wildPatch = true
		wildMinor = true
		ver = fmt.Sprintf("%s.0.0%s", m[3], m[6])
	} else if isX(strings.TrimPrefix(m[5], ".")) {
		wildPatch = true
		ver = fmt.Sprintf("%s%s.0%s", m[3], m[4], m[6])
	}

	v, err := NewVersion(ver)
	if err != nil {
		// The constraintRegex should catch any regex parsing errors. So,
		// we should never get here.
		return nil, errors.New("constraint Parser Error")
	}

	switch m[1] {
	case "^":
		// Caret always expands to a range
		return expandCaret(v), nil
	case "~":
		// Tilde always expands to a range
		return expandTilde(v, wildMinor), nil
	case "!=":
		// Not equals expands to a range if no element isX(); otherwise expands
		// to a union of ranges
		return expandNeq(v, wildMinor, wildPatch), nil
	case "", "=":
		if wildPatch || wildMinor {
			// Equalling a wildcard has the same behavior as expanding tilde
			return expandTilde(v, wildMinor), nil
		}
		return v, nil
	case ">":
		return expandGreater(v, false), nil
	case ">=", "=>":
		return expandGreater(v, true), nil
	case "<":
		return expandLess(v, wildMinor, wildPatch, false), nil
	case "<=", "=<":
		return expandLess(v, wildMinor, wildPatch, true), nil
	default:
		// Shouldn't be possible to get here, unless the regex is allowing
		// predicate we don't know about...
		return nil, fmt.Errorf("Unrecognized predicate %q", m[1])
	}
}

func expandCaret(v *Version) Constraint {
	maxv := &Version{
		major: v.major + 1,
		minor: 0,
		patch: 0,
	}

	return rangeConstraint{
		min:        v,
		max:        maxv,
		includeMin: true,
		includeMax: false,
	}
}

func expandTilde(v *Version, wildMinor bool) Constraint {
	if wildMinor {
		// When minor is wild on a tilde, behavior is same as caret
		return expandCaret(v)
	}

	maxv := &Version{
		major: v.major,
		minor: v.minor + 1,
		patch: 0,
	}

	return rangeConstraint{
		min:        v,
		max:        maxv,
		includeMin: true,
		includeMax: false,
	}
}

// expandNeq expands a "not-equals" constraint.
//
// If the constraint has any wildcards, it will expand into a unionConstraint
// (which is how we represent a disjoint set). If there are no wildcards, it
// will expand to a rangeConstraint with no min or max, but having the one
// exception.
func expandNeq(v *Version, wildMinor, wildPatch bool) Constraint {
	if !(wildMinor || wildPatch) {
		return rangeConstraint{
			excl: []*Version{v},
		}
	}

	// Create the low range with no min, and the max as the floor admitted by
	// the wildcard
	lr := rangeConstraint{
		max:        v,
		includeMax: false,
	}

	// The high range uses the derived version, bumped depending on where the
	// wildcards where, as the min, and is inclusive
	minv := &Version{
		major: v.major,
		minor: v.minor,
		patch: v.patch,
	}

	if wildMinor {
		minv.major++
	} else { // TODO should be an else if?
		minv.minor++
	}

	hr := rangeConstraint{
		min:        minv,
		includeMin: true,
	}

	return Union(lr, hr)
}

func expandGreater(v *Version, eq bool) Constraint {
	return rangeConstraint{
		min:        v,
		includeMin: eq,
	}
}

func expandLess(v *Version, wildMinor, wildPatch, eq bool) Constraint {
	v2 := &Version{
		major: v.major,
		minor: v.minor,
		patch: v.patch,
	}
	if wildMinor {
		v2.major++
	} else if wildPatch {
		v2.minor++
	}

	return rangeConstraint{
		max:        v2,
		includeMax: eq,
	}
}

// Constraint functions
func constraintNotEqual(v *Version, c *constraint) bool {
	if c.dirty {
		if c.con.Major() != v.Major() {
			return true
		}
		if c.con.Minor() != v.Minor() && !c.minorDirty {
			return true
		} else if c.minorDirty {
			return false
		}

		return false
	}

	return !v.Equal(c.con)
}

func constraintGreaterThan(v *Version, c *constraint) bool {
	return v.Compare(c.con) == 1
}

func constraintLessThan(v *Version, c *constraint) bool {
	if !c.dirty {
		return v.Compare(c.con) < 0
	}

	if v.Major() > c.con.Major() {
		return false
	} else if v.Minor() > c.con.Minor() && !c.minorDirty {
		return false
	}

	return true
}

func constraintGreaterThanEqual(v *Version, c *constraint) bool {
	return v.Compare(c.con) >= 0
}

func constraintLessThanEqual(v *Version, c *constraint) bool {
	if !c.dirty {
		return v.Compare(c.con) <= 0
	}

	if v.Major() > c.con.Major() {
		return false
	} else if v.Minor() > c.con.Minor() && !c.minorDirty {
		return false
	}

	return true
}

// ~*, ~>* --> >= 0.0.0 (any)
// ~2, ~2.x, ~2.x.x, ~>2, ~>2.x ~>2.x.x --> >=2.0.0, <3.0.0
// ~2.0, ~2.0.x, ~>2.0, ~>2.0.x --> >=2.0.0, <2.1.0
// ~1.2, ~1.2.x, ~>1.2, ~>1.2.x --> >=1.2.0, <1.3.0
// ~1.2.3, ~>1.2.3 --> >=1.2.3, <1.3.0
// ~1.2.0, ~>1.2.0 --> >=1.2.0, <1.3.0
func constraintTilde(v *Version, c *constraint) bool {
	if v.LessThan(c.con) {
		return false
	}

	// ~0.0.0 is a special case where all constraints are accepted. It's
	// equivalent to >= 0.0.0.
	if c.con.Major() == 0 && c.con.Minor() == 0 && c.con.Patch() == 0 {
		return true
	}

	if v.Major() != c.con.Major() {
		return false
	}

	if v.Minor() != c.con.Minor() && !c.minorDirty {
		return false
	}

	return true
}

// When there is a .x (dirty) status it automatically opts in to ~. Otherwise
// it's a straight =
func constraintTildeOrEqual(v *Version, c *constraint) bool {
	if c.dirty {
		c.msg = constraintMsg["~"]
		return constraintTilde(v, c)
	}

	return v.Equal(c.con)
}

// ^* --> (any)
// ^2, ^2.x, ^2.x.x --> >=2.0.0, <3.0.0
// ^2.0, ^2.0.x --> >=2.0.0, <3.0.0
// ^1.2, ^1.2.x --> >=1.2.0, <2.0.0
// ^1.2.3 --> >=1.2.3, <2.0.0
// ^1.2.0 --> >=1.2.0, <2.0.0
func constraintCaret(v *Version, c *constraint) bool {
	if v.LessThan(c.con) {
		return false
	}

	if v.Major() != c.con.Major() {
		return false
	}

	return true
}

type rwfunc func(i string) string

var constraintRangeRegex *regexp.Regexp

const cvRegex string = `v?([0-9|x|X|\*]+)(\.[0-9|x|X|\*]+)?(\.[0-9|x|X|\*]+)?` +
	`(-([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?` +
	`(\+([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?`

func isX(x string) bool {
	l := strings.ToLower(x)
	return l == "x" || l == "*"
}

func rewriteRange(i string) string {
	m := constraintRangeRegex.FindAllStringSubmatch(i, -1)
	if m == nil {
		return i
	}
	o := i
	for _, v := range m {
		t := fmt.Sprintf(">= %s, <= %s", v[1], v[11])
		o = strings.Replace(o, v[0], t, 1)
	}

	return o
}
