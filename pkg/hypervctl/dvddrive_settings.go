package hypervctl

const SyntheticDvdDriveType = "Microsoft:Hyper-V:Synthetic DVD Drive"

type SyntheticDvdDriveSettings struct {
	ResourceSettings
	systemSettings     *SystemSettings
	controllerSettings *ScsiControllerSettings
}

func (d *SyntheticDvdDriveSettings) DefineVirtualDvdDisk(imageFile string) (*VirtualDvdDiskStorageSettings, error) {
	vdvd := &VirtualDvdDiskStorageSettings{}

	if err := createDiskResourceInternal(d.systemSettings.S__PATH, d.S__PATH, imageFile, vdvd, VirtualDvdDiskType); err != nil {
		return nil, err
	}

	vdvd.driveSettings = d
	vdvd.systemSettings = d.systemSettings
	return vdvd, nil
}
