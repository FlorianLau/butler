package syscallex

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// JobObjectInfoClass
const (
	JobObjectInfoClass_JobObjectBasicProcessIdList       = 3
	JobObjectInfoClass_JobObjectExtendedLimitInformation = 9
)

// JobObjectBasicLimitInformation.LimitFlags
const (
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
)

const (
	CREATE_SUSPENDED      = 0x00000004
	PROCESS_ALL_ACCESS    = syscall.STANDARD_RIGHTS_REQUIRED | syscall.SYNCHRONIZE | 0xfff
	THREAD_SUSPEND_RESUME = 0x0002
)

type ThreadEntry32 struct {
	Size           uint32
	TUsage         uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCreateJobObject           = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject   = modkernel32.NewProc("SetInformationJobObject")
	procQueryInformationJobObject = modkernel32.NewProc("QueryInformationJobObject")
	procAssignProcessToJobObject  = modkernel32.NewProc("AssignProcessToJobObject")

	procGetCurrentThread    = modkernel32.NewProc("GetCurrentThread")
	procOpenThreadToken     = modkernel32.NewProc("OpenThreadToken")
	procGetDiskFreeSpaceExW = modkernel32.NewProc("GetDiskFreeSpaceExW")

	procOpenThread    = modkernel32.NewProc("OpenThread")
	procResumeThread  = modkernel32.NewProc("ResumeThread")
	procThread32First = modkernel32.NewProc("Thread32First")
	procThread32Next  = modkernel32.NewProc("Thread32Next")
)

func CreateJobObject(
	jobAttributes *syscall.SecurityAttributes,
	name *uint16,
) (handle syscall.Handle, err error) {
	r1, _, e1 := syscall.Syscall(
		procCreateJobObject.Addr(),
		2,
		uintptr(unsafe.Pointer(jobAttributes)),
		uintptr(unsafe.Pointer(name)),
		0,
	)
	handle = syscall.Handle(r1)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

type IoCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

func SetInformationJobObject(
	jobObject syscall.Handle,
	jobObjectInfoClass uint32,
	jobObjectInfo uintptr,
	jobObjectInfoLength uintptr,
) (err error) {
	r1, _, e1 := syscall.Syscall6(
		procSetInformationJobObject.Addr(),
		4,
		uintptr(jobObject),
		uintptr(jobObjectInfoClass),
		jobObjectInfo,
		jobObjectInfoLength,
		0, 0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func QueryInformationJobObject(
	jobObject syscall.Handle,
	jobObjectInfoClass uint32,
	jobObjectInfo uintptr,
	jobObjectInfoLength uintptr,
	returnLength uintptr,
) (err error) {
	r1, _, e1 := syscall.Syscall6(
		procQueryInformationJobObject.Addr(),
		5,
		uintptr(jobObject),
		uintptr(jobObjectInfoClass),
		jobObjectInfo,
		jobObjectInfoLength,
		returnLength,
		0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func AssignProcessToJobObject(
	jobObject syscall.Handle,
	process syscall.Handle,
) (err error) {
	r1, _, e1 := syscall.Syscall(
		procAssignProcessToJobObject.Addr(),
		2,
		uintptr(jobObject),
		uintptr(process),
		0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func GetCurrentThread() syscall.Handle {
	r1, _, _ := syscall.Syscall(
		procGetCurrentThread.Addr(),
		0,
		0, 0, 0,
	)
	return syscall.Handle(r1)
}

func OpenThreadToken(
	threadHandle syscall.Handle,
	desiredAccess uint32,
	openAsSelf uint32,
	tokenHandle *syscall.Token,
) (err error) {
	r1, _, e1 := syscall.Syscall6(
		procOpenThreadToken.Addr(),
		4,
		uintptr(threadHandle),
		uintptr(desiredAccess),
		uintptr(openAsSelf),
		uintptr(unsafe.Pointer(tokenHandle)),
		0, 0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

type DiskFreeSpace struct {
	FreeBytesAvailable     uint64
	TotalNumberOfBytes     uint64
	TotalNumberOfFreeBytes uint64
}

func GetDiskFreeSpaceEx(path *uint16) (dfs *DiskFreeSpace, err error) {
	var buf DiskFreeSpace
	dfs = &buf

	r1, _, e1 := syscall.Syscall6(
		procGetDiskFreeSpaceExW.Addr(),
		4,
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&buf.FreeBytesAvailable)),
		uintptr(unsafe.Pointer(&buf.TotalNumberOfBytes)),
		uintptr(unsafe.Pointer(&buf.TotalNumberOfFreeBytes)),
		0, 0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return dfs, err
}

func OpenThread(
	desiredAccess uint32,
	inheritHandle uint32,
	threadId uint32,
) (handle syscall.Handle, err error) {
	r1, _, e1 := syscall.Syscall(
		procOpenThread.Addr(),
		3,
		uintptr(desiredAccess),
		uintptr(inheritHandle),
		uintptr(threadId),
	)
	handle = syscall.Handle(r1)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func ResumeThread(
	thread syscall.Handle,
) (retCount uint32, err error) {
	r1, _, e1 := syscall.Syscall(
		procResumeThread.Addr(),
		1,
		uintptr(thread),
		0,
		0,
	)

	minusOne := int(-1)
	if r1 == uintptr(minusOne) {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	} else {
		retCount = uint32(r1)
	}
	return
}

func Thread32First(
	snapshot syscall.Handle,
	pThreadEntry *ThreadEntry32,
) (err error) {
	r1, _, e1 := syscall.Syscall(
		procThread32First.Addr(),
		2,
		uintptr(snapshot),
		uintptr(unsafe.Pointer(pThreadEntry)),
		0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func Thread32Next(
	snapshot syscall.Handle,
	pThreadEntry *ThreadEntry32,
) (err error) {
	r1, _, e1 := syscall.Syscall(
		procThread32Next.Addr(),
		2,
		uintptr(snapshot),
		uintptr(unsafe.Pointer(pThreadEntry)),
		0,
	)
	if r1 == 0 {
		if e1 != 0 {
			err = e1
		} else {
			err = syscall.EINVAL
		}
	}
	return
}
