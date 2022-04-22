package devicefinder

import (
	"testing"
)

func TestDeviceFound(t *testing.T) {
	filteredDevs, err := filterDevices(&[]string{"/dev/zigbee", "/dev/rtl_sdr"}, []string{"zigbee"})

	if len(filteredDevs) != 1 {
		t.Errorf("expected %v, got %v", 1, len(filteredDevs))
	}

	if err != nil {
		t.Error(err)
	}
}

func TestUnfilteredReduced(t *testing.T) {
	rawList := []string{"/dev/zigbee", "/dev/rtl_sdr"}

	initLen := len(rawList)

	filteredDevs, err := filterDevices(&rawList, []string{"zigbee"})

	filterLen := len(filteredDevs)
	unfilteredLen := len(rawList)

	if (filterLen + unfilteredLen) != initLen {
		t.Errorf("expected %v, got %v", initLen, (filterLen + unfilteredLen))
	}

	if err != nil {
		t.Error(err)
	}
}

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
		found := index(test.list, test.item) != -1
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
		list := remove(test.list, test.startIndex, test.count)

		count := len(list)
		if count != test.expectedCount {
			t.Errorf("Expected to find %d items after removel, found %d", count, test.expectedCount)
		}
	}
}
