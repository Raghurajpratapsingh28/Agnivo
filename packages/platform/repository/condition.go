package repository

import (
	"strconv"
	"strings"
)

// Condition is a composable, parameterized SQL predicate. Clauses use "?"
// placeholders internally; build renumbers them to PostgreSQL's $N form. Values
// are always bound as parameters, never interpolated, so conditions are safe
// against injection as long as column names come from repository code (not user
// input).
type Condition struct {
	clause string
	args   []any
}

// Raw wraps a hand-written predicate using "?" placeholders, e.g.
// Raw("created_at > ?", since).
func Raw(clause string, args ...any) Condition {
	return Condition{clause: clause, args: args}
}

// Eq builds "col = ?".
func Eq(col string, val any) Condition { return Condition{clause: col + " = ?", args: []any{val}} }

// Neq builds "col <> ?".
func Neq(col string, val any) Condition { return Condition{clause: col + " <> ?", args: []any{val}} }

// Gt builds "col > ?".
func Gt(col string, val any) Condition { return Condition{clause: col + " > ?", args: []any{val}} }

// Lt builds "col < ?".
func Lt(col string, val any) Condition { return Condition{clause: col + " < ?", args: []any{val}} }

// In builds "col IN (?, ?, ...)". An empty set yields a always-false predicate
// so callers need not special-case it.
func In(col string, vals []any) Condition {
	if len(vals) == 0 {
		return Condition{clause: "FALSE"}
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(vals)), ", ")
	return Condition{clause: col + " IN (" + ph + ")", args: vals}
}

// And combines conditions with AND, skipping empty ones.
func And(conds ...Condition) Condition { return combine("AND", conds) }

// Or combines conditions with OR, skipping empty ones.
func Or(conds ...Condition) Condition { return combine("OR", conds) }

func combine(op string, conds []Condition) Condition {
	parts := make([]string, 0, len(conds))
	var args []any
	for _, c := range conds {
		if c.empty() {
			continue
		}
		parts = append(parts, "("+c.clause+")")
		args = append(args, c.args...)
	}
	if len(parts) == 0 {
		return Condition{}
	}
	return Condition{clause: strings.Join(parts, " "+op+" "), args: args}
}

func (c Condition) empty() bool { return strings.TrimSpace(c.clause) == "" }

// build renumbers "?" placeholders to "$start, $start+1, ..." and returns the
// final clause with its ordered args.
func (c Condition) build(start int) (string, []any) {
	if c.empty() {
		return "", nil
	}
	var b strings.Builder
	b.Grow(len(c.clause) + 4)
	n := start
	for i := 0; i < len(c.clause); i++ {
		if c.clause[i] == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			n++
			continue
		}
		b.WriteByte(c.clause[i])
	}
	return b.String(), c.args
}
