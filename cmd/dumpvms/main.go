//go:build windows
// +build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/drtimf/wmi"
	"github.com/n1hility/hypervctl/pkg/hypervctl"
	"github.com/n1hility/hypervctl/pkg/wmiext"
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

	fmt.Printf(string(b))

	fmt.Println("======")

	path, err := vmms.DefineSystem("testmeout")
	if err != nil {
		panic(err)
	}
 
	fmt.Println("Created: " + path)
	

	var service *wmi.Service
	if service, err = wmi.NewLocalService(hypervctl.HyperVNamespace); err != nil {
		panic(err) 
	}


	defer service.Close()

	systemSetting, err := wmiext.FindFirstRelatedInstance(service, path, "Msvm_VirtualSystemSettingData")
	if err != nil {
		panic(err)
	}

	settingPath, err := wmiext.ConvertToPath(systemSetting)
	if err != nil {
		panic(err)
	}

	resourceSetting, err := wmiext.FindFirstRelatedInstance(service, settingPath, "Msvm_ResourceAllocationSettingData")
	if err != nil {
		panic(err)
	}

	resourcePath, err := wmiext.ConvertToPath(resourceSetting)
	fmt.Println(resourcePath)

}
