package array

import (
	"github.com/golang/glog"
)

func Index(s []string, v string) int {
	for i, vs := range s {
		if v == vs {
			glog.V(3).Infof("Found %s at %d", v, i)
			return i
		}
	}

	glog.V(3).Infof("Did not find %s", v)

	return -1
}

func Remove(s []string, startIndex int, count int) []string {
	if count <= 0 {
		glog.Warningf("Removed no items, startIndex: %d, count: %d", startIndex, count)
		return s
	}

	if startIndex < 0 {
		glog.Warningf("Trying to remove starting at index %d", startIndex)

		endIndex := startIndex + count

		if endIndex <= 0 {
			glog.Warningf("Removed no items, startIndex: %d, count: %d", startIndex, count)
			return s
		}

		if endIndex > 0 {
			glog.Warningf("Removed %d items, startIndex: %d, count: %d", endIndex+1, startIndex, count)
			return s[endIndex:]
		}
	}

	if startIndex >= len(s) {
		glog.Warningf("Removed no items, startIndex: %d, count: %d", startIndex, count)
		return s
	}

	if startIndex+count > len(s) {
		glog.Warningf("Removed %d items, startIndex: %d, count: %d", len(s)-startIndex+1, startIndex, count)
		return s[:startIndex]
	}

	glog.V(3).Infof("Removed %d items, startIndex: %d", count, startIndex)

	return append(s[:startIndex], s[startIndex+count:]...)
}
