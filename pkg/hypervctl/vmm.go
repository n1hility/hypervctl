package hypervctl

import (
	"fmt"

	"github.com/drtimf/wmi"
	"github.com/n1hility/hypervctl/pkg/wmiext"
)

const (
	HyperVNamespace                = "root\\virtualization\\v2"
	VirtualSystemManagementService = "Msvm_VirtualSystemManagementService"
)

type VirtualMachineManager struct {
}

func (*VirtualMachineManager) GetAll() ([]*VirtualMachine, error) {
	const wql = "Select * From Msvm_ComputerSystem"

	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return [](*VirtualMachine){}, err
	}
	defer service.Close()

	var enum *wmi.Enum
	if enum, err = service.ExecQuery(wql); err != nil {
		return nil, err
	}
	defer enum.Close()

	var vms [](*VirtualMachine)
	for {
		vm := &VirtualMachine{}
		done, err := wmiext.NextObjectWithPath(enum, vm)
		if err != nil {
			return vms, err
		}
		if done {
			break
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

func (*VirtualMachineManager) GetMachine(name string) (*VirtualMachine, error) {
	const wql = "Select * From Msvm_ComputerSystem Where ElementName='%s'"

	vm := &VirtualMachine{}
	var service *wmi.Service
	var err error

	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return vm, err
	}
	defer service.Close()

	var enum *wmi.Enum
	if enum, err = service.ExecQuery(fmt.Sprintf(wql, name)); err != nil {
		return nil, err
	}
	defer enum.Close()

	done, err := wmiext.NextObjectWithPath(enum, vm)
	if err != nil {
		return vm, err
	}

	if done {
		return vm, fmt.Errorf("Could not find virtual machine %q", name)
	}

	return vm, nil
}
