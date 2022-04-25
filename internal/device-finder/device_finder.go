package devicefinder

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	cfg "github.com/geoff-coppertop/device-manager-plugin/internal/config"
	log "github.com/sirupsen/logrus"
)

type DeviceMap struct {
	Paths []string
	Group string
}

func GenerateDeviceMapping(cfg cfg.Config) ([]DeviceMap, error) {
	var devMap []DeviceMap

	rawDeviceMap := make(map[string][]string)

	for _, devMatch := range cfg.Matchers {
		/* Have we seen this root path before? If not find all devices, and symlinks to devices, off of
		 * that root path and keep track of it */
		if _, ok := rawDeviceMap[devMatch.Search]; !ok {
			devs, err := findDevices(devMatch.Search)
			if err != nil {
				/* Didn't find anything under this root path go to next matcher */
				continue
			}

			rawDeviceMap[devMatch.Search] = devs
		}

		/* Filter the devices, this will reduce the list of unfiltered by returing the ones which match
		 * the expression */
		unfilteredDevs := rawDeviceMap[devMatch.Search]
		filteredDevs, err := filterDevices(&unfilteredDevs, devMatch.Match)
		if err != nil {
			/* Something failed in the match checking */
			continue
		}
		if len(filteredDevs) <= 0 {
			/* We didn't find any devices which match the expression */
			continue
		}

		/* Need to follow symlinks that may have been pulled in to the filtered list and remove where
		 * they point from the unfiltered list if applicable */
		err = filterSymlinks(&unfilteredDevs, filteredDevs)

		rawDeviceMap[devMatch.Search] = unfilteredDevs

		if devMatch.Group != "" {
			group := sanitizeName(devMatch.Group)

			devMap = append(devMap, DeviceMap{Paths: filteredDevs, Group: group})
		} else {
			for _, dev := range filteredDevs {
				group := sanitizeName(strings.TrimPrefix(dev, devMatch.Search))

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
				log.Warn(err)
				return nil
			}

			switch mode := info.Mode(); {
			case mode&fs.ModeSymlink != 0:
				log.Tracef("Following symlink: %s", path)
				// Found symlink, follow it and see if it's pointing at a device directly
				symPath, err := os.Readlink(path)
				if err != nil {
					log.Warnf("Bad symlink: %v", err)
					return nil
				}

				if !filepath.IsAbs(symPath) {
					symPath = filepath.Join(filepath.Dir(path), symPath)
					log.Tracef("Sympath: %s", symPath)
				}

				info, err = os.Stat(symPath)
				if err != nil {
					log.Warnf("Bad stat: %v", err)
					return nil
				}

				if info.Mode()&fs.ModeDevice != 0 {
					devices = append(devices, path)
				} else {
					log.Tracef("Ignoring: %s", path)
				}

			case mode&fs.ModeDevice != 0:
				devices = append(devices, path)
			default:
				log.Tracef("Ignoring: %s", path)
			}

			return nil
		})
	if err != nil {
		log.Warnf("Directory walk failed: %v", err)
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
		info, err := os.Stat(dev)
		if err != nil {
			log.Warnf("Bad stat: %v", err)
			return err
		}

		if (info.Mode() & fs.ModeSymlink) != 0 {
			log.Tracef("Following symlink: %s", dev)
			// Found symlink, follow it and see if it's pointing at a device directly
			symPath, err := os.Readlink(dev)
			if err != nil {
				log.Warnf("Bad symlink: %v", err)
				return err
			}

			if !filepath.IsAbs(symPath) {
				symPath = filepath.Join(filepath.Dir(dev), symPath)
				log.Tracef("Sympath: %s", symPath)
			}

			if i := index(*unfilteredDevs, symPath); i != -1 {
				/* remove from the unfilteredDevs list because the filtered list has a symlink that points
				 * at this device and it and the symlink are potentially going to be linked into a
				 * container */
				*unfilteredDevs = remove(*unfilteredDevs, i, 1)
			}
		}
	}

	return nil
}

func index(s []string, v string) int {
	for i, vs := range s {
		if v == vs {
			return i
		}
	}

	return -1
}

func remove(s []string, startIndex int, count int) []string {
	if count <= 0 {
		return s
	}

	if startIndex < 0 {
		if startIndex+count <= 0 {
			return s
		}

		if startIndex+count > 0 {
			return s[startIndex+count:]
		}
	}

	if startIndex >= len(s) {
		return s
	}

	if startIndex+count > len(s) {
		return s[:startIndex]
	}

	return append(s[:startIndex], s[startIndex+count:]...)
}
