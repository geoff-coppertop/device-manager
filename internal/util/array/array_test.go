package array

import (
	"testing"
)

func TestIndex(t *testing.T) {
	var tests = []struct {
		list  []string
		item  string
		found bool
	}{
		{[]string{"apple", "banana", "grapes"}, "orange", false},
		{[]string{"apple", "banana", "grapes"}, "apple", true},
		{[]string{"apple", "banana", "grapes"}, "banana", true},
		{[]string{"apple", "banana", "grapes"}, "grapes", true},
	}

	for _, test := range tests {
		found := Index(test.list, test.item) != -1
		if test.found != found {
			format := "Expected to find %s, in %v"
			if !test.found {
				format = "Not expected to find %s, in %v"
			}

			t.Errorf(format, test.item, test.list)
		}
	}
}

func TestRemove(t *testing.T) {
	var tests = []struct {
		list          []string
		startIndex    int
		count         int
		expectedCount int
	}{
		{[]string{"apple", "banana", "grapes"}, -1, -1, 3},
		{[]string{"apple", "banana", "grapes"}, -1, 0, 3},
		{[]string{"apple", "banana", "grapes"}, -1, 1, 3},
		{[]string{"apple", "banana", "grapes"}, -1, 2, 2},
		{[]string{"apple", "banana", "grapes"}, 3, 1, 3},
		{[]string{"apple", "banana", "grapes"}, 2, 1, 2},
		{[]string{"apple", "banana", "grapes"}, 2, 2, 2},
	}

	for _, test := range tests {
		list := Remove(test.list, test.startIndex, test.count)

		count := len(list)
		if count != test.expectedCount {
			t.Errorf("Expected to find %d items after removel, found %d", count, test.expectedCount)
		}
	}
}
