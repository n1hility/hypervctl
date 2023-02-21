package hypervctl

import (
	"github.com/drtimf/wmi"
	"github.com/n1hility/hypervctl/pkg/wmiext"
)

const SyntheticDiskDriveType = "Microsoft:Hyper-V:Synthetic Disk Drive"

type SyntheticDiskDriveSettings struct {
	ResourceSettings
	systemSettings     *SystemSettings
	controllerSettings *ScsiControllerSettings
}

type diskAssociation interface {
	setParent(parent string)
	setHostResource(resource []string)
}

func (d *SyntheticDiskDriveSettings) DefineVirtualHardDisk(vhdxFile string) (*VirtualHardDiskStorageSettings, error) {
	vhd := &VirtualHardDiskStorageSettings{}

	if err := createDiskResourceInternal(d.systemSettings.S__PATH, d.S__PATH, vhdxFile, vhd, VirtualHardDiskType); err != nil {
		return nil, err
	}

	vhd.driveSettings = d
	vhd.systemSettings = d.systemSettings
	return vhd, nil
}

func createDiskResourceInternal(systemPath string, drivePath string, file string, settings diskAssociation, resourceType string) error {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return err
	}
	defer service.Close()

	if err = populateDefaults(resourceType, settings); err != nil {
		return err
	}

	settings.setHostResource([]string{file})
	settings.setParent(drivePath)

	diskResource, err := createResourceSettingGeneric(settings, resourceType)
	if err != nil {
		return err
	}

	path, err := addResource(service, systemPath, diskResource)
	if err != nil {
		return err
	}

	return wmiext.GetObjectAsObject(service, path, settings)
}
