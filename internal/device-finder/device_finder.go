package devicefinder

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/glog"

	cfg "github.com/geoff-coppertop/device-manager-plugin/internal/config"
	arr "github.com/geoff-coppertop/device-manager-plugin/internal/util/array"
	fs "github.com/geoff-coppertop/device-manager-plugin/internal/util/fs"
)

type DeviceMap struct {
	Paths []string
	Group string
}

func GenerateDeviceMapping(cfg cfg.Config) ([]DeviceMap, error) {
	glog.V(3).Info("Entering GenerateDeviceMapping")

	var devMap []DeviceMap

	rawDeviceMap := make(map[string][]string)

	for _, devMatch := range cfg.Matchers {
		/* Have we seen this root path before? If not find all devices, and symlinks to devices, off of
		 * that root path and keep track of it */
		if _, ok := rawDeviceMap[devMatch.Search]; !ok {
			glog.V(2).Infof("New root path: %s", devMatch.Search)
			devs, err := findDevices(devMatch.Search)
			if err != nil {
				glog.V(2).Infof("Didn't find any devices under: %s", devMatch.Search)
				/* Didn't find anything under this root path go to next matcher */
				continue
			}

			glog.V(2).Infof("Found %d devices under: %s", len(devs), devMatch.Search)

			rawDeviceMap[devMatch.Search] = devs
		}

		/* Filter the devices, this will reduce the list of unfiltered by returing the ones which match
		 * the expression */
		unfilteredDevs := rawDeviceMap[devMatch.Search]
		filteredDevs, err := filterDevices(&unfilteredDevs, devMatch.Match)
		if err != nil {
			/* Something failed in the match checking */
			glog.Error(err)
			continue
		}

		unfilteredDevsLen := len(unfilteredDevs)

		glog.V(2).Infof(
			"filterDevices returned %d, for %s. %d devices remaining",
			len(filteredDevs),
			devMatch.Match,
			unfilteredDevsLen)

		if len(filteredDevs) <= 0 {
			glog.V(2).Infof("No devices found which match the expression")
			continue
		}

		/* Need to follow symlinks that may have been pulled in to the filtered list and remove where
		 * they point from the unfiltered list if applicable */
		err = filterSymlinks(&unfilteredDevs, filteredDevs)

		glog.V(2).Infof(
			"filterSymlinks removed %d device(s)",
			unfilteredDevsLen-len(unfilteredDevs))

		rawDeviceMap[devMatch.Search] = unfilteredDevs

		if devMatch.Group != "" {
			group := sanitizeName(devMatch.Group)

			glog.V(2).Infof("%d devices in group: %s", len(filteredDevs), group)

			devMap = append(devMap, DeviceMap{Paths: filteredDevs, Group: group})
		} else {
			for _, dev := range filteredDevs {
				group := sanitizeName(strings.TrimPrefix(dev, devMatch.Search))

				glog.V(2).Infof("%d devices in group: %s", 1, group)

				devMap = append(devMap, DeviceMap{Paths: []string{dev}, Group: group})
			}
		}
	}

	return devMap, nil
}

func sanitizeName(name string) string {
	return strings.Replace(name, "/", "-", -1)
}

func findDevices(root string) (devices []string, err error) {
	err = filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				glog.Warning(err)
				return nil
			}

			if fs.IsSymlink(path) {
				symPath, err := fs.FollowSymlink(path)
				if err != nil {
					glog.Warning(err)
					return nil
				}

				path = symPath
			}

			if fs.IsDevice(path) {
				glog.V(2).Infof("Adding, %s to list.", path)
				devices = append(devices, path)
			} else {
				glog.V(3).Infof("Ignoring: %s", path)
			}

			return nil
		})

	if err != nil {
		glog.Warning(err)
		return nil, err
	}

	return devices, nil
}

func filterDevices(unfilteredDevices *[]string, patterns []string) (filteredDevices []string, err error) {
	for _, pattern := range patterns {
		temp := (*unfilteredDevices)[:0]
		for _, device := range *unfilteredDevices {
			res, err := regexp.MatchString(pattern, device)
			if err != nil {
				return nil, err
			}
			if res {
				filteredDevices = append(filteredDevices, device)
			} else {
				temp = append(temp, device)
			}
		}
		*unfilteredDevices = temp
	}
	return filteredDevices, err
}

func filterSymlinks(unfilteredDevs *[]string, filteredDevs []string) error {
	for _, dev := range filteredDevs {
		glog.V(3).Infof("filterSymlinks: %s", dev)

		if fs.IsSymlink(dev) {
			symPath, err := fs.FollowSymlink(dev)
			if err != nil {
				glog.Warning(err)
				return nil
			}

			if i := arr.Index(*unfilteredDevs, symPath); i != -1 {
				glog.V(3).Infof("removing %s from unfilteredDevs", symPath)
				/* remove from the unfilteredDevs list because the filtered list has a symlink that points
				 * at this device and it and the symlink are potentially going to be linked into a
				 * container */
				*unfilteredDevs = arr.Remove(*unfilteredDevs, i, 1)
			}
		}
	}

	return nil
}
