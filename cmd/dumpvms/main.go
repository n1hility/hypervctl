//go:build windows
// +build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/n1hility/hypervctl/pkg/hypervctl"
)

func main() {
	var err error

	vmms := hypervctl.VirtualMachineManager{}

	vms, err := vmms.GetAll()
	if err != nil {
		fmt.Printf("Could not retrieve virtual machines : %s\n", err.Error())
		os.Exit(1)
	}

	b, err := json.MarshalIndent(vms, "", "\t")

	if err != nil {
		fmt.Println("Failed to generate output")
		os.Exit(1)
	}

	fmt.Println(string(b))

	fmt.Println("======")

	// path, err := vmms.DefineSystem("testmeout")
	// if err != nil {
	// 	panic(err)
	// }

	// fmt.Println("Created: " + path)

	// var service *wmi.Service
	// if service, err = wmi.NewLocalService(hypervctl.HyperVNamespace); err != nil {
	// 	panic(err)
	// }

	// defer service.Close()

	// controller, err := hypervctl.CreateScsiController(path)
	// if err != nil {
	// 	panic(err)
	// }

	vhdxFile := `C:\Users\Jason\foo.vhdx`
	isoFile := `C:\Users\Jason\bar.iso`

	// drive, err := hypervctl.CreateSyntheticDvdDrive(path, controller, 0)
	// if err != nil {
	// 	panic(err)
	// }

	// dvdPath, err := hypervctl.CreateVirtualDvdDisk(path, drive, isoFile)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println("Dvd drive path = " + dvdPath)

	// drive, err = hypervctl.CreateSyntheticDiskDrive(path, controller, 1)
	// if err != nil {
	// 	panic(err)
	// }

	// _ = os.Remove(vhdxFile)

	// err = hypervctl.CreateVhdxFile(vhdxFile, 1024*1024*1024*10)
	// if err != nil {
	// 	panic(err)
	// }

	// fmt.Printf("Drive path = %s\n", drive)
	// diskPath, err := hypervctl.CreateVirtualHardDisk(path, drive, vhdxFile)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println("Hard drive path = " + diskPath)

	// port, err:= hypervctl.CreateSyntheticEthernetPort(path)
	// if err != nil {
	// 	panic(err)
	// }

	// _, err = hypervctl.CreateEthernetPortConnection(path, port, "")
	// if err != nil {
	// 	panic(err)
	// }

	builder := hypervctl.NewSystemSettingsBuilder()

	// System

	_ = builder.PrepareSystemSettings("nextgen")
	memorySettings, err := builder.PrepareMemorySettings()
	if err != nil {
		panic(err)
	}

	memorySettings.VirtualQuantity = 8192 // Startup memory
	memorySettings.Reservation = 1024     // min
	memorySettings.Limit = 16384          // max

	processor, err := builder.PrepareProcessorSettings()
	if err != nil {
		panic(err)
	}

	processor.VirtualQuantity = 4 // 4 cores

	systemSettings, err := builder.Build()
	if err != nil {
		panic(err)

	}

	// Disks

	controller, err := systemSettings.AddScsiController()
	if err != nil {
		panic(err)
	}

	diskDrive, err := controller.AddSyntheticDiskDrive(0)
	if err != nil {
		panic(err)
	}

	_, err = diskDrive.DefineVirtualHardDisk(vhdxFile)
	if err != nil {
		panic(err)
	}

	dvdDrive, err := controller.AddSyntheticDvdDrive(1)
	if err != nil {
		panic(err)
	}

	_, err = dvdDrive.DefineVirtualDvdDisk(isoFile)
	if err != nil {
		panic(err)
	}

	// Network

	port, err := systemSettings.AddSyntheticEthernetPort()
	if err != nil {
		panic(err)
	}

	_, err = port.DefineEthernetPortConnection("")
	if err != nil {
		panic(err)
	}

	vm, err := systemSettings.GetVM()
	if err != nil {
		panic(err)
	}

	fmt.Println(vm.S__PATH)

	fmt.Println("Done!")
}
