//go:build windows

package plugin

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var windowsJobs sync.Map
var ntResumeProcess = syscall.NewLazyDLL("ntdll.dll").NewProc("NtResumeProcess")

func prepareProcessGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("configure job object: %w", err)
	}
	windowsJobs.Store(cmd, job)
	return nil
}

func attachProcessGroup(cmd *exec.Cmd) error {
	value, ok := windowsJobs.Load(cmd)
	if !ok {
		return fmt.Errorf("job object not prepared")
	}
	job := value.(windows.Handle)
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("open plugin process: %w", err)
	}
	err = windows.AssignProcessToJobObject(job, process)
	if err == nil {
		status, _, callErr := ntResumeProcess.Call(uintptr(process))
		if status != 0 {
			err = fmt.Errorf("resume plugin process: status=%d: %v", status, callErr)
		}
	}
	_ = windows.CloseHandle(process)
	if err != nil {
		return fmt.Errorf("assign/resume plugin process: %w", err)
	}
	return nil
}

func cleanupProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		terminateProcessTree(cmd.Process.Pid)
	}
	if value, ok := windowsJobs.LoadAndDelete(cmd); ok {
		job := value.(windows.Handle)
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
	}
}

func terminateProcessTree(root int) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return
	}
	entries := []windows.ProcessEntry32{entry}
	for {
		entry = windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}
	targets := map[uint32]bool{uint32(root): true}
	changed := true
	for changed {
		changed = false
		for _, process := range entries {
			if !targets[process.ProcessID] && targets[process.ParentProcessID] {
				targets[process.ProcessID] = true
				changed = true
			}
		}
	}
	for pid := range targets {
		if pid == uint32(root) {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(process, 1)
		_ = windows.CloseHandle(process)
	}
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		terminateProcessTree(cmd.Process.Pid)
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
	if value, ok := windowsJobs.Load(cmd); ok {
		_ = windows.TerminateJobObject(value.(windows.Handle), 1)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
