package slicesx_test

import (
	"strconv"
	"testing"

	"github.com/agnivo/agnivo/packages/platform/slicesx"
	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	out := slicesx.Map([]int{1, 2, 3}, strconv.Itoa)
	assert.Equal(t, []string{"1", "2", "3"}, out)
	assert.Nil(t, slicesx.Map[int, int](nil, func(i int) int { return i }))
}

func TestFilter(t *testing.T) {
	out := slicesx.Filter([]int{1, 2, 3, 4}, func(i int) bool { return i%2 == 0 })
	assert.Equal(t, []int{2, 4}, out)
}

func TestReduce(t *testing.T) {
	sum := slicesx.Reduce([]int{1, 2, 3}, 0, func(a, v int) int { return a + v })
	assert.Equal(t, 6, sum)
}

func TestContainsAndUnique(t *testing.T) {
	assert.True(t, slicesx.Contains([]string{"a", "b"}, "b"))
	assert.False(t, slicesx.Contains([]string{"a"}, "z"))
	assert.Equal(t, []int{1, 2, 3}, slicesx.Unique([]int{1, 1, 2, 3, 2}))
}

func TestChunk(t *testing.T) {
	assert.Equal(t, [][]int{{1, 2}, {3, 4}, {5}}, slicesx.Chunk([]int{1, 2, 3, 4, 5}, 2))
	assert.Equal(t, [][]int{{1, 2}}, slicesx.Chunk([]int{1, 2}, 0))
}

func TestKeyByAndGroupBy(t *testing.T) {
	type u struct {
		id   int
		team string
	}
	users := []u{{1, "a"}, {2, "b"}, {3, "a"}}
	byID := slicesx.KeyBy(users, func(x u) int { return x.id })
	assert.Equal(t, "b", byID[2].team)
	byTeam := slicesx.GroupBy(users, func(x u) string { return x.team })
	assert.Len(t, byTeam["a"], 2)
	assert.Len(t, byTeam["b"], 1)
}
