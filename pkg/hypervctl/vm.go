//go:build windows
// +build windows

package hypervctl

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"time"

	"github.com/drtimf/wmi"
	"github.com/n1hility/hypervctl/pkg/wmiext"
)

const (
	KvpOperationFailed    = 32768
	KvpAccessDenied       = 32769
	KvpNotSupported       = 32770
	KvpStatusUnknown      = 32771
	KvpTimeoutOcurred     = 32772
	KvpIllegalArgument    = 32773
	KvpSystemInUse        = 32774
	KvpInvalidState       = 32775
	KvpIncorrectDataType  = 32776
	KvpSystemNotAvailable = 32777
	KvpOutOfMemory        = 32778
	KvpNotFound           = 32779

	HyperVNamespace                = "root\\virtualization\\v2"
	VirtualSystemManagementService = "Msvm_VirtualSystemManagementService"
	KvpExchangeDataItemName        = "Msvm_KvpExchangeDataItem"
	MemorySettingDataName          = "Msvm_MemorySettingData"
)

type VirtualMachineManager struct {
}

type VirtualMachine struct {
	S__PATH                                  string `json:"-"`
	S__CLASS                                 string `json:"-"`
	InstanceID                               string
	Caption                                  string
	Description                              string
	ElementName                              string
	InstallDate                              string
	OperationalStatus                        []uint16
	StatusDescriptions                       []string
	Status                                   string
	HealthState                              uint16
	CommunicationStatus                      uint16
	DetailedStatus                           uint16
	OperatingStatus                          uint16
	PrimaryStatus                            uint16
	EnabledState                             uint16
	OtherEnabledState                        string
	RequestedState                           uint16
	EnabledDefault                           uint16
	TimeOfLastStateChange                    string
	AvailableRequestedStates                 []uint16
	TransitioningToState                     uint16
	CreationClassName                        string
	Name                                     string
	PrimaryOwnerName                         string
	PrimaryOwnerContact                      string
	Roles                                    []string
	NameFormat                               string
	OtherIdentifyingInfo                     []string
	IdentifyingDescriptions                  []string
	Dedicated                                []uint16
	OtherDedicatedDescriptions               []string
	ResetCapability                          uint16
	PowerManagementCapabilities              []uint16
	OnTimeInMilliseconds                     uint64
	ProcessID                                uint32
	TimeOfLastConfigurationChange            string
	NumberOfNumaNodes                        uint16
	ReplicationState                         uint16
	ReplicationHealth                        uint16
	ReplicationMode                          uint16
	FailedOverReplicationType                uint16
	LastReplicationType                      uint16
	LastApplicationConsistentReplicationTime string
	LastReplicationTime                      string
	LastSuccessfulBackupTime                 string
	EnhancedSessionModeState                 uint16
}

type CimKvpItems struct {
	Instances []CimKvpItem `xml:"INSTANCE"`
}

type CimKvpItem struct {
	Properties []CimKvpItemProperty `xml:"PROPERTY"`
}

type CimKvpItemProperty struct {
	Name  string `xml:"NAME,attr"`
	Value string `xml:"VALUE"`
}

type KvpError struct {
	ErrorCode int
	message   string
}

func (k *KvpError) Error() string {
	return fmt.Sprintf("%s (%d)", k.message, k.ErrorCode)
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

func (vm *VirtualMachine) AddKeyValuePair(key string, value string) error {
	return vm.kvpOperation("AddKvpItems", key, value, "key already exists?")
}

func (vm *VirtualMachine) ModifyKeyValuePair(key string, value string) error {
	return vm.kvpOperation("ModifyKvpItems", key, value, "key invalid?")
}

func (vm *VirtualMachine) PutKeyValuePair(key string, value string) error {
	err := vm.AddKeyValuePair(key, value)
	kvpError, ok := err.(*KvpError)
	if !ok || kvpError.ErrorCode != KvpIllegalArgument {
		return err
	}

	return vm.ModifyKeyValuePair(key, value)
}

func (vm *VirtualMachine) RemoveKeyValuePair(key string) error {
	return vm.kvpOperation("RemoveKvpItems", key, "", "key invalid?")
}

func (vm *VirtualMachine) GetKeyValuePairs() (map[string]string, error) {
	var service *wmi.Service
	var err error

	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return nil, err
	}

	defer service.Close()

	i, err := wmiext.FindFirstRelatedInstance(service, vm.S__PATH, "Msvm_KvpExchangeComponent")
	if err != nil {
		return nil, err
	}

	defer i.Close()

	var path string
	path, err = i.GetPropertyAsString("__PATH")
	if err != nil {
		return nil, err

	}

	i, err = wmiext.FindFirstRelatedInstance(service, path, "Msvm_KvpExchangeComponentSettingData")
	if err != nil {
		return nil, err
	}
	defer i.Close()

	s, err := i.GetPropertyAsString("HostExchangeItems")
	if err != nil {
		return nil, err
	}

	// Workaround XML decoder's inability to handle multiple root elements
	r := io.MultiReader(
		strings.NewReader("<root>"),
		strings.NewReader(s),
		strings.NewReader("</root>"),
	)

	var items CimKvpItems
	if err = xml.NewDecoder(r).Decode(&items); err != nil {
		return nil, err
	}

	ret := make(map[string]string)
	for _, item := range items.Instances {
		var key, value string
		for _, prop := range item.Properties {
			if strings.EqualFold(prop.Name, "Name") {
				key = prop.Value
			} else if strings.EqualFold(prop.Name, "Data") {
				value = prop.Value
			}
		}
		if len(key) > 0 {
			ret[key] = value
		}
	}

	return ret, nil
}

func (vm *VirtualMachine) kvpOperation(op string, key string, value string, illegalSuggestion string) error {
	var service *wmi.Service
	var vsms, job *wmi.Instance
	var err error

	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return (err)
	}
	defer service.Close()

	vsms, err = wmiext.GetSingletonInstance(service, VirtualSystemManagementService)
	if err != nil {
		return err
	}
	defer vsms.Close()

	itemStr := createItem(service, key, value)

	execution := wmiext.BeginInvoke(service, vsms, op).
		Set("TargetSystem", vm.S__PATH).
		Set("DataItems", []string{itemStr}).
		Execute()

	if err := execution.Get("Job", &job).End(); err != nil {
		return fmt.Errorf("%s execution failed: %w", op, err)
	}

	err = translateError(wmiext.WaitJob(service, job), illegalSuggestion)
	defer job.Close()
	return err
}

func createItem(service *wmi.Service, key string, value string) string {
	item, err := wmiext.SpawnInstance(service, KvpExchangeDataItemName)
	if err != nil {
		panic(err)
	}
	defer item.Close()

	item.Put("Name", key)
	item.Put("Data", value)
	item.Put("Source", 0)
	itemStr := wmiext.GetCimText(item)
	return itemStr
}

type MemorySettings struct {
	S__PATH                    string
	InstanceID                 string
	Caption                    string // = "Memory Default Settings"
	Description                string // = "Describes the default settings for the memory resources."
	ElementName                string
	ResourceType               uint16 // = 4
	OtherResourceType          string
	ResourceSubType            string // = "Microsoft:Hyper-V:Memory"
	PoolID                     string
	ConsumerVisibility         uint16
	HostResource               []string
	HugePagesEnabled           bool
	AllocationUnits            string // = "byte * 2^20"
	VirtualQuantity            uint64
	Reservation                uint64
	Limit                      uint64
	Weight                     uint32
	AutomaticAllocation        bool // = True
	AutomaticDeallocation      bool // = True
	Parent                     string
	Connection                 []string
	Address                    string
	MappingBehavior            uint16
	AddressOnParent            string
	VirtualQuantityUnits       string // = "byte * 2^20"
	DynamicMemoryEnabled       bool
	TargetMemoryBuffer         uint32
	IsVirtualized              bool // = True
	SwapFilesInUse             bool
	MaxMemoryBlocksPerNumaNode uint64
	SgxSize                    uint64
	SgxEnabled                 bool
}

func FetchDefaultMemorySettings() (*MemorySettings, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return nil, err
	}
	defer service.Close()

	settings := &MemorySettings{}
	return settings, populateDefaults(service, "Microsoft:Hyper-V:Memory", settings)
}

func FetchDefaultProcessorSettings() (*MemorySettings, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return nil, err
	}
	defer service.Close()

	settings := &MemorySettings{}
	return settings, populateDefaults(service, "Microsoft:Hyper-V:Processor", settings)
}

func FetchDefaultResourceSettings(resourceType string) (*ResourceSettings, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return nil, err
	}
	defer service.Close()

	settings := &ResourceSettings{}
	return settings, populateDefaults(service, resourceType, settings)
}

func populateDefaults(service *wmi.Service, subType string, target interface{}) error {
	ref, err := findResourceDefaults(service, subType)
	if err != nil {
		return err
	}

	return wmiext.GetObjectAsObject(service, ref, target)
}

func findResourceDefaults(service *wmi.Service, subType string) (string, error) {
	wql := fmt.Sprintf("SELECT * FROM Msvm_AllocationCapabilities WHERE ResourceSubType = '%s'", subType)
	instance, err := wmiext.FindFirstInstance(service, wql)
	if err != nil {
		return "", err
	}
	defer instance.Close()

	path, err := wmiext.ConvertToPath(instance)
	if err != nil {
		return "", err
	}

	enum, err := service.ExecQuery(fmt.Sprintf("references of {%s} where ResultClass = Msvm_SettingsDefineCapabilities", path))
	if err != nil {
		return "", err
	}
	defer enum.Close()

	for {
		entry, err := enum.Next()
		if err != nil {
			return "", err
		}
		if entry == nil {
			return "", errors.New("Could not find settings definition for resource")
		}

		value, vErr := wmiext.GetPropertyAsUint(entry, "ValueRole")
		ref, pErr := entry.GetPropertyAsString("PartComponent")
		entry.Close()
		if vErr == nil && pErr == nil && value == 0 {
			return ref, nil
		}
	}
}

func CreateMemorySettings(settings *MemorySettings) (string, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return "", err
	}

	ref, err := findResourceDefaults(service, "Microsoft:Hyper-V:Memory")
	if err != nil {
		return "", err
	}

	memory, err := service.GetObject(ref)
	if err != nil {
		return "", err
	}

	defer memory.Close()
	memory, err = wmiext.CloneInstance(memory)
	if err != nil {
		return "", err
	}
	defer memory.Close()

	if err = wmiext.InstancePutAll(memory, settings); err != nil {
		return "", err
	}

	return wmiext.GetCimText(memory), nil
}

func CreateResourceSettings(settings *ResourceSettings, resourceType string) (string, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return "", err
	}

	ref, err := findResourceDefaults(service, resourceType)
	if err != nil {
		return "", err
	}

	resource, err := service.GetObject(ref)
	if err != nil {
		return "", err
	}

	defer resource.Close()
	resource, err = wmiext.CloneInstance(resource)
	if err != nil {
		return "", err
	}
	defer resource.Close()

	if err = wmiext.InstancePutAll(resource, settings); err != nil {
		return "", err
	}

	return wmiext.GetCimText(resource), nil
}

func CreateProcessorSettings(settings *MemorySettings) (string, error) {
	var service *wmi.Service
	var err error
	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return "", err
	}

	ref, err := findResourceDefaults(service, "Microsoft:Hyper-V:Processor")
	if err != nil {
		return "", err
	}

	memory, err := service.GetObject(ref)
	if err != nil {
		return "", err
	}

	defer memory.Close()
	memory, err = wmiext.CloneInstance(memory)
	if err != nil {
		return "", err
	}
	defer memory.Close()

	if err = wmiext.InstancePutAll(memory, settings); err != nil {
		return "", err
	}

	return wmiext.GetCimText(memory), nil
}

/*
AllocationUnits                : percent / 1000
AllowACountMCount              : True
AutomaticAllocation            : True
AutomaticDeallocation          : True
Caption                        : Processor
Connection                     :
ConsumerVisibility             : 3
CpuGroupId                     : 00000000-0000-0000-0000-000000000000
Description                    : Settings for Microsoft Virtual Processor.
DisableSpeculationControls     : False
ElementName                    : Processor
EnableHostResourceProtection   : False
EnableLegacyApicMode           : False
EnablePageShattering           : 0
EnablePerfmonIpt               : False
EnablePerfmonLbr               : False
EnablePerfmonPebs              : False
EnablePerfmonPmu               : False
ExposeVirtualizationExtensions : False
HideHypervisorPresent          : False
HostResource                   :
HwThreadsPerCore               : 0
InstanceID                     : Microsoft:B5314955-3924-42BA-ABF9-793993D340A0\b637f346-6a0e-4dec-af52-bd70cb80a21d\0
Limit                          : 100000
LimitCPUID                     : False
LimitProcessorFeatures         : False
MappingBehavior                :
MaxNumaNodesPerSocket          : 1
MaxProcessorsPerNumaNode       : 4
OtherResourceType              :
Parent                         :
PoolID                         :
Reservation                    : 0
ResourceSubType                : Microsoft:Hyper-V:Processor
ResourceType                   : 3
VirtualQuantity                : 2
VirtualQuantityUnits           : count
Weight                         : 100
PSComputerName                 : JASONGREENEEE6B
*/

type ProcessorSettings struct {
	S__PATH                        string
	InstanceID                     string
	Caption                        string // = "Processor"
	Description                    string // = "A logical processor of the hypervisor running on the host computer system."
	ElementName                    string
	ResourceType                   uint16 // = 3
	OtherResourceType              string
	ResourceSubType                string // = "Microsoft:Hyper-V:Processor"
	PoolID                         string
	ConsumerVisibility             uint16
	HostResource                   []string
	AllocationUnits                string // = "percent / 1000"
	VirtualQuantity                uint64 // = "count"
	Reservation                    uint64 // = 0
	Limit                          uint64 // = 100000
	Weight                         uint32 // = 100
	AutomaticAllocation            bool   // = True
	AutomaticDeallocation          bool   // = True
	Parent                         string
	Connection                     []string
	Address                        string
	MappingBehavior                uint16
	AddressOnParent                string
	VirtualQuantityUnits           string // = "count"
	LimitCPUID                     bool
	HwThreadsPerCore               uint64
	LimitProcessorFeatures         bool
	uint64                         uint64 // MaxNumaNodesPerSocket
	EnableHostResourceProtection   bool
	CpuGroupId                     string
	HideHypervisorPresent          bool
	ExposeVirtualizationExtensions bool
}

type SystemSettings struct {
	S__PATH                              string
	InstanceID                           string
	Caption                              string // = "Virtual Machine Settings"
	Description                          string
	ElementName                          string
	VirtualSystemIdentifier              string
	VirtualSystemType                    string
	Notes                                []string
	CreationTime                         time.Time
	ConfigurationID                      string
	ConfigurationDataRoot                string
	ConfigurationFile                    string
	SnapshotDataRoot                     string
	SuspendDataRoot                      string
	SwapFileDataRoot                     string
	LogDataRoot                          string
	AutomaticStartupAction               uint16 // non-zero
	AutomaticStartupActionDelay          time.Time
	AutomaticStartupActionSequenceNumber uint16
	AutomaticShutdownAction              uint16 // non-zero
	AutomaticRecoveryAction              uint16 // non-zero
	RecoveryFile                         string
	BIOSGUID                             string
	BIOSSerialNumber                     string
	BaseBoardSerialNumber                string
	ChassisSerialNumber                  string
	Architecture                         string
	ChassisAssetTag                      string
	BIOSNumLock                          bool
	BootOrder                            []uint16
	Parent                               string
	UserSnapshotType                     uint16 // non-zero
	IsSaved                              bool
	AdditionalRecoveryInformation        string
	AllowFullSCSICommandSet              bool
	DebugChannelId                       uint32
	DebugPortEnabled                     uint16
	DebugPort                            uint32
	Version                              string
	IncrementalBackupEnabled             bool
	VirtualNumaEnabled                   bool
	AllowReducedFcRedundancy             bool // = False
	VirtualSystemSubType                 string
	BootSourceOrder                      []string
	PauseAfterBootFailure                bool
	NetworkBootPreferredProtocol         uint16 // non-zero
	GuestControlledCacheTypes            bool
	AutomaticSnapshotsEnabled            bool
	IsAutomaticSnapshot                  bool
	GuestStateFile                       string
	GuestStateDataRoot                   string
	LockOnDisconnect                     bool
	ParentPackage                        string
	AutomaticCriticalErrorActionTimeout  time.Time
	AutomaticCriticalErrorAction         uint16
	ConsoleMode                          uint16
	SecureBootEnabled                    bool
	SecureBootTemplateId                 string
	LowMmioGapSize                       uint64
	HighMmioGapSize                      uint64
	EnhancedSessionTransportType         uint16
}

func DefaultSystemSettings() *SystemSettings {
	return &SystemSettings{
		// setup all non-zero settings
		AutomaticStartupAction:       2,    // no auto-start
		AutomaticShutdownAction:      4,    // shutdown
		AutomaticRecoveryAction:      3,    // restart
		UserSnapshotType:             2,    // no snapshotting
		NetworkBootPreferredProtocol: 4096, // ipv4 for pxe
		VirtualSystemSubType:         "Microsoft:Hyper-V:SubType:2",
	}

}

type ResourceSettings struct {
	Caption               string
	Description           string
	InstanceID            string
	ElementName           string
	ResourceType          uint16
	OtherResourceType     string
	ResourceSubType       string
	PoolID                string
	ConsumerVisibility    uint16
	HostResource          []string
	AllocationUnits       string
	VirtualQuantity       uint64
	Reservation           uint64
	Limit                 uint64
	Weight                uint32
	AutomaticAllocation   bool
	AutomaticDeallocation bool
	Parent                string
	Connection            []string
	Address               string
	MappingBehavior       uint16
	AddressOnParent       string
	VirtualQuantityUnits  string // = "count"
}

type VirtualHardDiskSettings struct {
	InstanceID                 string
	Caption                    string // = "Virtual Hard Disk Setting Data"
	Description                string // = "Setting Data for a Virtual Hard Disk"
	ElementName                string
	Type                       uint16
	Format                     uint16
	Path                       string
	ParentPath                 string
	ParentTimestamp            time.Time
	ParentIdentifier           string
	MaxInternalSize            uint64
	BlockSize                  uint32
	LogicalSectorSize          uint32
	PhysicalSectorSize         uint32
	VirtualDiskId              string
	DataAlignment              uint64
	PmemAddressAbstractionType uint16
	IsPmemCompatible           bool
}

func (vmms *VirtualMachineManager) DefineSystem(name string) (string, error) {
	var service *wmi.Service
	var err error

	if service, err = wmi.NewLocalService(HyperVNamespace); err != nil {
		return "", err
	}
	defer service.Close()

	systemSettings := DefaultSystemSettings()
	if err != nil {
		return "", err
	}
	systemSettings.ElementName = name
	systemSettingsInst, err := wmiext.SpawnInstance(service, "Msvm_VirtualSystemSettingData")
	if err != nil {
		return "", err
	}
	defer systemSettingsInst.Close()

	err = wmiext.InstancePutAll(systemSettingsInst, systemSettings)
	if err != nil {
		return "", err
	}

	memorySettings, err := FetchDefaultMemorySettings()
	if err != nil {
		return "", err

	}

	memoryStr, err := CreateMemorySettings(memorySettings)
	if err != nil {
		return "", err
	}

	processorSettings, err := FetchDefaultProcessorSettings()
	if err != nil {
		return "", err
	}

	processorStr, err := CreateProcessorSettings(processorSettings)
	if err != nil {
		return "", err
	}

	vsms, err := wmiext.GetSingletonInstance(service, VirtualSystemManagementService)
	if err != nil {
		return "", err
	}
	defer vsms.Close()

	systemStr := wmiext.GetCimText(systemSettingsInst)

	var job *wmi.Instance
	var res int32
	var resultingSystem string
	err = wmiext.BeginInvoke(service, vsms, "DefineSystem").
		Set("SystemSettings", systemStr).
		Set("ResourceSettings", []string{memoryStr, processorStr}).
		Execute().
		Get("Job", &job).
		Get("ResultingSystem", &resultingSystem).
		Get("ReturnValue", &res).End()

	if err != nil {
		return "", fmt.Errorf("Failed to define system: %w", err)
	}

	err = waitVMResult(res, service, job)

	return resultingSystem, err
}

func waitVMResult(res int32, service *wmi.Service, job *wmi.Instance) error {
	var err error

	if res == 4096 {
		err = wmiext.WaitJob(service, job)
		defer job.Close()
	}

	if err != nil {
		desc, _ := job.GetPropertyAsString("ErrorDescription")
		desc = strings.Replace(desc, "\n", " ", -1)
		return fmt.Errorf("Failed to define system: %w (%s)", err, desc)
	}

	return err
}

func CreateHardDrive(systemPath string) error {
	const scsiControllerType = "Microsoft:Hyper-V:Synthetic SCSI Controller"
	const virtualHardDriveType = "Microsoft:Hyper-V:Synthetic SCSI Controller"

	var service *wmi.Service
	if service, err := wmi.NewLocalService(HyperVNamespace); err != nil {
		return err
	}
	defer service.Close()

	systemSetting, err := wmiext.FindFirstRelatedInstance(service, systemPath, "Msvm_VirtualSystemSettingData")
	if err != nil {
		return err
	}

	settingPath, err := wmiext.ConvertToPath(systemSetting)
	if err != nil {
		return err
	}

	resourceStr, err := createResourceFromDefault(scsiControllerType)
	if err != nil {
		return err
	}

	scsiControllerPath, err := addResource(service, settingPath, resourceStr)
	if err != nil {
		return err
	}

	vhdSettings, err := FetchDefaultResourceSettings(virtualHardDriveType)
	if err != nil {
		return err
	}

	vhdSettings.Parent = scsiControllerPath
	vhdSettings.AddressOnParent = "0"

	//vhdPath, err := CreateResourceSettings(vhdSettings, virtualHardDriveType)
	if err != nil {
		return err
	}

	fmt.Println(service)

}

func createHardDiskSettings(service *wmi.Service) error {
	instance, err := wmiext.SpawnInstance(service, "Msvm_VirtualHardDiskSettingData")
	if err != nil {
		return err
	}
	defer instance.Close()

	//settings := &VirtualHardDiskSettings{}

}

func createResourceFromDefault(resourceType string) (string, error) {
	resourceSettings, err := FetchDefaultResourceSettings(resourceType)
	if err != nil {
		return "", err
	}

	resourceStr, err := CreateResourceSettings(resourceSettings, resourceType)
	if err != nil {
		return "", err
	}

	return resourceStr, nil
}

func addResource(service *wmi.Service, systemSettingPath string, resourceSettings string) (string, error) {
	vsms, err := wmiext.GetSingletonInstance(service, VirtualSystemManagementService)
	if err != nil {
		return "", err
	}
	defer vsms.Close()

	var res int32
	var resultingSettings []string
	var job *wmi.Instance
	err = wmiext.BeginInvoke(service, vsms, "AddResourceSettings").
		Set("AffectedConfiguration", systemPath).
		Set("ResourceSettings", []string{resourceSettings}).
		Execute().
		Get("Job", &job).
		Get("ResultingResourceSettings", &resultingSettings).
		Get("ReturnValue", &res).End()

	err = waitVMResult(res, service, job)

	if len(resultingSettings) > 0 {
		return resultingSettings[0], err
	}

	return "", err
}

func translateError(source error, illegalSuggestion string) error {
	j, ok := source.(*wmiext.JobError)

	if !ok {
		return source
	}

	var message string
	switch j.ErrorCode {
	case KvpOperationFailed:
		message = "Operation failed"
	case KvpAccessDenied:
		message = "Access denied"
	case KvpNotSupported:
		message = "Not supported"
	case KvpStatusUnknown:
		message = "Status is unknown"
	case KvpTimeoutOcurred:
		message = "Timeout occurred"
	case KvpIllegalArgument:
		message = "Illegal argument (" + illegalSuggestion + ")"
	case KvpSystemInUse:
		message = "System is in use"
	case KvpInvalidState:
		message = "Invalid state for this operation"
	case KvpIncorrectDataType:
		message = "Incorrect data type"
	case KvpSystemNotAvailable:
		message = "System is not available"
	case KvpOutOfMemory:
		message = "Out of memory"
	case KvpNotFound:
		message = "Not found"
	default:
		return source
	}

	return &KvpError{j.ErrorCode, message}
}
