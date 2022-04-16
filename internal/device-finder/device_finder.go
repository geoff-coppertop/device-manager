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

	for _, devMatch := range cfg.Matchers {
		devs, err := findDevices(devMatch.Search)
		if err != nil {
			return []DeviceMap{}, err
		}

		filteredDevs, err := filterDevices(devs, devMatch.Match)
		if err != nil {
			return []DeviceMap{}, err
		}

		if devMatch.Group != "" {
			group := sanitizeName(devMatch.Group)

			devMap = append(devMap, DeviceMap{Paths: filteredDevs, Group: group})
		} else {
			for _, dev := range filteredDevs {
				group := sanitizeName(strings.TrimPrefix(dev, "/dev"))

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

func filterDevices(unfilteredDevices []string, patterns []string) (filteredDevices []string, err error) {
	for _, pattern := range patterns {
		temp := unfilteredDevices[:0]
		for _, device := range unfilteredDevices {
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
		unfilteredDevices = temp
	}
	return filteredDevices, err
}
