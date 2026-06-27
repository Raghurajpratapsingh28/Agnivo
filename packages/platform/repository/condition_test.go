package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConditionBuildRenumbers(t *testing.T) {
	c := And(Eq("name", "web"), Gt("age", 18))
	clause, args := c.build(1)
	assert.Equal(t, "(name = $1) AND (age > $2)", clause)
	assert.Equal(t, []any{"web", 18}, args)
}

func TestConditionBuildOffset(t *testing.T) {
	c := Eq("id", "x")
	clause, args := c.build(5)
	assert.Equal(t, "id = $5", clause)
	assert.Equal(t, []any{"x"}, args)
}

func TestInCondition(t *testing.T) {
	c := In("status", []any{"a", "b", "c"})
	clause, args := c.build(1)
	assert.Equal(t, "status IN ($1, $2, $3)", clause)
	assert.Len(t, args, 3)
}

func TestInEmptyIsFalse(t *testing.T) {
	c := In("status", nil)
	clause, _ := c.build(1)
	assert.Equal(t, "FALSE", clause)
}

func TestOrAndEmptySkipped(t *testing.T) {
	c := And(Condition{}, Eq("a", 1), Condition{})
	clause, args := c.build(1)
	assert.Equal(t, "(a = $1)", clause)
	assert.Len(t, args, 1)

	assert.True(t, Condition{}.empty())
	assert.True(t, And().empty())
}

func TestBuildSetDeterministic(t *testing.T) {
	set, args := buildSet(map[string]any{"b": 2, "a": 1, "c": 3}, 0)
	assert.Equal(t, "a = $1, b = $2, c = $3", set)
	assert.Equal(t, []any{1, 2, 3}, args)
}

func TestSplitValuesDeterministic(t *testing.T) {
	cols, ph, args := splitValues(map[string]any{"z": 1, "a": 2}, 0)
	assert.Equal(t, []string{"a", "z"}, cols)
	assert.Equal(t, []string{"$1", "$2"}, ph)
	assert.Equal(t, []any{2, 1}, args)
}
