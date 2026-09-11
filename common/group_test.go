package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitGroupList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"spaces only", "  ,  ", []string{}},
		{"single", "default", []string{"default"}},
		{"multi", "group0,group1", []string{"group0", "group1"}},
		{"trims whitespace", " group0 , group1 ", []string{"group0", "group1"}},
		{"drops empties", "group0,,group1,", []string{"group0", "group1"}},
		{"dedupes preserving order", "b,a,b", []string{"b", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SplitGroupList(tt.input))
		})
	}
}

func TestPrimaryGroup(t *testing.T) {
	assert.Equal(t, "group0", PrimaryGroup("group0,group1"))
	assert.Equal(t, "group0", PrimaryGroup(" group0 , group1 "))
	assert.Equal(t, "group0", PrimaryGroup("group0"))
	assert.Equal(t, "default", PrimaryGroup(""))
	assert.Equal(t, "default", PrimaryGroup(" , "))
}

func TestNormalizeGroupList(t *testing.T) {
	assert.Equal(t, "group0,group1", NormalizeGroupList(" group0 , , group1 "))
	assert.Equal(t, "group0", NormalizeGroupList("group0,group0"))
	assert.Equal(t, "default", NormalizeGroupList(""))
}
