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
