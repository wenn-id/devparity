//go:build windows

package execute

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	jobObjectExtendedLimitInformationClass = 9
	jobObjectLimitKillOnJobClose           = 0x2000
	processSetQuota                        = 0x0100
	processTerminate                       = 0x0001
	processSuspendResume                   = 0x0800
	createSuspended                        = 0x00000004
)

type windowsProcessTree struct {
	job syscall.Handle
}

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObject          = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	terminateJobObject       = kernel32.NewProc("TerminateJobObject")
	ntdll                    = syscall.NewLazyDLL("ntdll.dll")
	ntResumeProcess          = ntdll.NewProc("NtResumeProcess")
)

func configureHostProcessTree(command *exec.Cmd) (hostProcessTree, error) {
	job, _, err := createJobObject.Call(0, 0)
	if job == 0 {
		return nil, err
	}
	tree := &windowsProcessTree{job: syscall.Handle(job)}
	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	result, _, err := setInformationJobObject.Call(
		job,
		jobObjectExtendedLimitInformationClass,
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if result == 0 {
		_ = tree.Close()
		return nil, err
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createSuspended}
	return tree, nil
}

func (tree *windowsProcessTree) Attach(process *os.Process) error {
	processHandle, err := syscall.OpenProcess(processSetQuota|processTerminate|processSuspendResume, false, uint32(process.Pid))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(processHandle)
	result, _, callErr := assignProcessToJobObject.Call(uintptr(tree.job), uintptr(processHandle))
	if result == 0 {
		return callErr
	}
	status, _, _ := ntResumeProcess.Call(uintptr(processHandle))
	if status != 0 {
		return fmt.Errorf("NtResumeProcess failed with status 0x%x", status)
	}
	return nil
}

func (tree *windowsProcessTree) Cancel(process *os.Process) error {
	if process == nil {
		return errors.New("host process is not started")
	}
	result, _, err := terminateJobObject.Call(uintptr(tree.job), 1)
	if result == 0 && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (tree *windowsProcessTree) Close() error {
	if tree.job == 0 {
		return nil
	}
	err := syscall.CloseHandle(tree.job)
	tree.job = 0
	return err
}
