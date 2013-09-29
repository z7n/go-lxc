/*
 * container.go: Go bindings for lxc
 *
 * Copyright © 2013, S.Çağlar Onur
 *
 * Authors:
 * S.Çağlar Onur <caglar@10ur.org>
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.

 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.

 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301  USA
 */

package lxc

// #include <lxc/lxc.h>
// #include <lxc/lxccontainer.h>
// #include "lxc.h"
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Container struct
type Container struct {
	container *C.struct_lxc_container
	verbosity Verbosity
	sync.RWMutex
}

func (lxc *Container) ensureDefinedAndRunning() error {
	if !lxc.Defined() {
		return fmt.Errorf(errNotDefined, C.GoString(lxc.container.name))
	}

	if !lxc.Running() {
		return fmt.Errorf(errNotRunning, C.GoString(lxc.container.name))
	}
	return nil
}

func (lxc *Container) ensureDefinedButNotRunning() error {
	if !lxc.Defined() {
		return fmt.Errorf(errNotDefined, C.GoString(lxc.container.name))
	}

	if lxc.Running() {
		return fmt.Errorf(errAlreadyRunning, C.GoString(lxc.container.name))
	}
	return nil
}

// Name returns container's name
func (lxc *Container) Name() string {
	lxc.RLock()
	defer lxc.RUnlock()

	return C.GoString(lxc.container.name)
}

// Defined returns whether the container is already defined or not
func (lxc *Container) Defined() bool {
	lxc.RLock()
	defer lxc.RUnlock()

	return bool(C.lxc_container_defined(lxc.container))
}

// Running returns whether the container is already running or not
func (lxc *Container) Running() bool {
	lxc.RLock()
	defer lxc.RUnlock()

	return bool(C.lxc_container_running(lxc.container))
}

// State returns the container's state
func (lxc *Container) State() State {
	lxc.RLock()
	defer lxc.RUnlock()

	return stateMap[C.GoString(C.lxc_container_state(lxc.container))]
}

// InitPID returns the container's PID
func (lxc *Container) InitPID() int {
	lxc.RLock()
	defer lxc.RUnlock()

	return int(C.lxc_container_init_pid(lxc.container))
}

// Daemonize returns whether the daemonize flag is set
func (lxc *Container) Daemonize() bool {
	lxc.RLock()
	defer lxc.RUnlock()

	return bool(lxc.container.daemonize != 0)
}

// SetDaemonize sets the daemonize flag
func (lxc *Container) SetDaemonize() error {
	lxc.Lock()
	defer lxc.Unlock()

	C.lxc_container_want_daemonize(lxc.container)
	if bool(lxc.container.daemonize == 0) {
		return fmt.Errorf(errDaemonizeFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// SetCloseAllFds sets the close_all_fds flag for the container
func (lxc *Container) SetCloseAllFds() error {
	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_want_close_all_fds(lxc.container)) {
		return fmt.Errorf(errCloseAllFdsFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// SetVerbosity sets the verbosity level of some API calls
func (lxc *Container) SetVerbosity(verbosity Verbosity) {
	lxc.Lock()
	defer lxc.Unlock()

	lxc.verbosity = verbosity
}

// Freeze freezes the running container
func (lxc *Container) Freeze() error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	if lxc.State() == FROZEN {
		return fmt.Errorf(errAlreadyFrozen, C.GoString(lxc.container.name))
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_freeze(lxc.container)) {
		return fmt.Errorf(errFreezeFailed, C.GoString(lxc.container.name))
	}

	return nil
}

// Unfreeze unfreezes the frozen container
func (lxc *Container) Unfreeze() error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	if lxc.State() != FROZEN {
		return fmt.Errorf(errNotFrozen, C.GoString(lxc.container.name))
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_unfreeze(lxc.container)) {
		return fmt.Errorf(errUnfreezeFailed, C.GoString(lxc.container.name))
	}

	return nil
}

// Create creates the container using given template and arguments
func (lxc *Container) Create(template string, args ...string) error {
	if lxc.Defined() {
		return fmt.Errorf(errAlreadyDefined, C.GoString(lxc.container.name))
	}

	lxc.Lock()
	defer lxc.Unlock()

	ctemplate := C.CString(template)
	defer C.free(unsafe.Pointer(ctemplate))

	ret := false
	if args != nil {
		cargs := makeArgs(args)
		defer freeArgs(cargs, len(args))

		ret = bool(C.lxc_container_create(lxc.container, ctemplate, C.int(lxc.verbosity), cargs))
	} else {
		ret = bool(C.lxc_container_create(lxc.container, ctemplate, C.int(lxc.verbosity), nil))
	}

	if !ret {
		return fmt.Errorf(errCreateFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Start starts the container
func (lxc *Container) Start(useinit bool, args ...string) error {
	if err := lxc.ensureDefinedButNotRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	ret := false

	cuseinit := 0
	if useinit {
		cuseinit = 1
	}

	if args != nil {
		cargs := makeArgs(args)
		defer freeArgs(cargs, len(args))

		ret = bool(C.lxc_container_start(lxc.container, C.int(cuseinit), cargs))
	} else {
		ret = bool(C.lxc_container_start(lxc.container, C.int(cuseinit), nil))
	}

	if !ret {
		return fmt.Errorf(errStartFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Stop stops the container
func (lxc *Container) Stop() error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_stop(lxc.container)) {
		return fmt.Errorf(errStopFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Reboot reboots the container
func (lxc *Container) Reboot() error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_reboot(lxc.container)) {
		return fmt.Errorf(errRebootFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Shutdown shutdowns the container
func (lxc *Container) Shutdown(timeout int) error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_shutdown(lxc.container, C.int(timeout))) {
		return fmt.Errorf(errShutdownFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Destroy destroys the container
func (lxc *Container) Destroy() error {
	if err := lxc.ensureDefinedButNotRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_destroy(lxc.container)) {
		return fmt.Errorf(errDestroyFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Clone clones the container
func (lxc *Container) Clone(name string, flags int, backend BackendStore) error {
	if err := lxc.ensureDefinedButNotRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	if !bool(C.lxc_container_clone(lxc.container, cname, C.int(flags), C.CString(backend.String()))) {
		return fmt.Errorf(errCloneFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// CloneToDirectory clones the container using Directory backendstore
func (lxc *Container) CloneToDirectory(name string) error {
	return lxc.Clone(name, 0, Directory)
}

// CloneToOverlayFS clones the container using OverlayFS backendstore
func (lxc *Container) CloneToOverlayFS(name string) error {
	return lxc.Clone(name, CloneSnapshot, OverlayFS)
}

// Wait waits till the container changes its state or timeouts
func (lxc *Container) Wait(state State, timeout int) bool {
	lxc.Lock()
	defer lxc.Unlock()

	cstate := C.CString(state.String())
	defer C.free(unsafe.Pointer(cstate))

	return bool(C.lxc_container_wait(lxc.container, cstate, C.int(timeout)))
}

// ConfigFileName returns the container's configuration file's name
func (lxc *Container) ConfigFileName() string {
	lxc.RLock()
	defer lxc.RUnlock()

	// allocated in lxc.c
	configFileName := C.lxc_container_config_file_name(lxc.container)
	defer C.free(unsafe.Pointer(configFileName))

	return C.GoString(configFileName)
}

// ConfigItem returns the value of the given key
func (lxc *Container) ConfigItem(key string) []string {
	lxc.RLock()
	defer lxc.RUnlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	// allocated in lxc.c
	configItem := C.lxc_container_get_config_item(lxc.container, ckey)
	defer C.free(unsafe.Pointer(configItem))

	ret := strings.TrimSpace(C.GoString(configItem))
	return strings.Split(ret, "\n")
}

// SetConfigItem sets the value of given key
func (lxc *Container) SetConfigItem(key string, value string) error {
	lxc.Lock()
	defer lxc.Unlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))

	if !bool(C.lxc_container_set_config_item(lxc.container, ckey, cvalue)) {
		return fmt.Errorf(errSettingConfigItemFailed, C.GoString(lxc.container.name), key, value)
	}
	return nil
}

// CgroupItem returns the value of the given key
func (lxc *Container) CgroupItem(key string) []string {
	lxc.RLock()
	defer lxc.RUnlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	// allocated in lxc.c
	cgroupItem := C.lxc_container_get_cgroup_item(lxc.container, ckey)
	defer C.free(unsafe.Pointer(cgroupItem))

	ret := strings.TrimSpace(C.GoString(cgroupItem))
	return strings.Split(ret, "\n")
}

// SetCgroupItem sets the value of given key
func (lxc *Container) SetCgroupItem(key string, value string) error {
	lxc.Lock()
	defer lxc.Unlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	cvalue := C.CString(value)
	defer C.free(unsafe.Pointer(cvalue))

	if !bool(C.lxc_container_set_cgroup_item(lxc.container, ckey, cvalue)) {
		return fmt.Errorf(errSettingCgroupItemFailed, C.GoString(lxc.container.name), key, value)
	}
	return nil
}

// ClearConfigItem clears the value of given key
func (lxc *Container) ClearConfigItem(key string) error {
	lxc.Lock()
	defer lxc.Unlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	if !bool(C.lxc_container_clear_config_item(lxc.container, ckey)) {
		return fmt.Errorf(errClearingCgroupItemFailed, C.GoString(lxc.container.name), key)
	}
	return nil
}

// Keys returns the keys
func (lxc *Container) Keys(key string) []string {
	lxc.RLock()
	defer lxc.RUnlock()

	ckey := C.CString(key)
	defer C.free(unsafe.Pointer(ckey))

	// allocated in lxc.c
	keys := C.lxc_container_get_keys(lxc.container, ckey)
	defer C.free(unsafe.Pointer(keys))

	ret := strings.TrimSpace(C.GoString(keys))
	return strings.Split(ret, "\n")
}

// LoadConfigFile loads the configuration file from given path
func (lxc *Container) LoadConfigFile(path string) error {
	lxc.Lock()
	defer lxc.Unlock()

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if !bool(C.lxc_container_load_config(lxc.container, cpath)) {
		return fmt.Errorf(errLoadConfigFailed, C.GoString(lxc.container.name), path)
	}
	return nil
}

// SaveConfigFile saves the configuration file to given path
func (lxc *Container) SaveConfigFile(path string) error {
	lxc.Lock()
	defer lxc.Unlock()

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if !bool(C.lxc_container_save_config(lxc.container, cpath)) {
		return fmt.Errorf(errSaveConfigFailed, C.GoString(lxc.container.name), path)
	}
	return nil
}

// ConfigPath returns the configuration file's path
func (lxc *Container) ConfigPath() string {
	lxc.RLock()
	defer lxc.RUnlock()

	return C.GoString(C.lxc_container_get_config_path(lxc.container))
}

// SetConfigPath sets the configuration file's path
func (lxc *Container) SetConfigPath(path string) error {
	lxc.Lock()
	defer lxc.Unlock()

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	if !bool(C.lxc_container_set_config_path(lxc.container, cpath)) {
		return fmt.Errorf(errSettingConfigPathFailed, C.GoString(lxc.container.name), path)
	}
	return nil
}

// MemoryUsageInBytes returns memory usage in bytes
func (lxc *Container) MemoryUsageInBytes() (ByteSize, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	memUsed, err := strconv.ParseFloat(lxc.CgroupItem("memory.usage_in_bytes")[0], 64)
	if err != nil {
		return -1, err
	}
	return ByteSize(memUsed), err
}

// SwapUsageInBytes returns swap usage in bytes
func (lxc *Container) SwapUsageInBytes() (ByteSize, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	swapUsed, err := strconv.ParseFloat(lxc.CgroupItem("memory.memsw.usage_in_bytes")[0], 64)
	if err != nil {
		return -1, err
	}
	return ByteSize(swapUsed), err
}

// MemoryLimitInBytes returns memory limit in bytes
func (lxc *Container) MemoryLimitInBytes() (ByteSize, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	memLimit, err := strconv.ParseFloat(lxc.CgroupItem("memory.limit_in_bytes")[0], 64)
	if err != nil {
		return -1, err
	}
	return ByteSize(memLimit), err
}

// SetMemoryLimitInBytes sets memory limit in bytes
func (lxc *Container) SetMemoryLimitInBytes(limit ByteSize) error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	if err := lxc.SetCgroupItem("memory.limit_in_bytes", limit.ConvertToString()); err != nil {
		return fmt.Errorf(errSettingMemoryLimitFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// SwapLimitInBytes returns the swap limit in bytes
func (lxc *Container) SwapLimitInBytes() (ByteSize, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	swapLimit, err := strconv.ParseFloat(lxc.CgroupItem("memory.memsw.limit_in_bytes")[0], 64)
	if err != nil {
		return -1, err
	}
	return ByteSize(swapLimit), err
}

// SetSwapLimitInBytes sets memory limit in bytes
func (lxc *Container) SetSwapLimitInBytes(limit ByteSize) error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	if err := lxc.SetCgroupItem("memory.memsw.limit_in_bytes", limit.ConvertToString()); err != nil {
		return fmt.Errorf(errSettingSwapLimitFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// CPUTime returns the total CPU time (in nanoseconds) consumed by all tasks in this cgroup (including tasks lower in the hierarchy).
func (lxc *Container) CPUTime() (time.Duration, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	cpuUsage, err := strconv.ParseInt(lxc.CgroupItem("cpuacct.usage")[0], 10, 64)
	if err != nil {
		return -1, err
	}
	return time.Duration(cpuUsage), err
}

// CPUTimePerCPU returns the CPU time (in nanoseconds) consumed on each CPU by all tasks in this cgroup (including tasks lower in the hierarchy).
func (lxc *Container) CPUTimePerCPU() ([]time.Duration, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	var cpuTimes []time.Duration

	for _, v := range strings.Split(lxc.CgroupItem("cpuacct.usage_percpu")[0], " ") {
		cpuUsage, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		cpuTimes = append(cpuTimes, time.Duration(cpuUsage))
	}
	return cpuTimes, nil
}

// CPUStats returns the number of CPU cycles (in the units defined by USER_HZ on the system) consumed by tasks in this cgroup and its children in both user mode and system (kernel) mode.
func (lxc *Container) CPUStats() ([]int64, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	cpuStat := lxc.CgroupItem("cpuacct.stat")
	user, err := strconv.ParseInt(strings.Split(cpuStat[0], " ")[1], 10, 64)
	if err != nil {
		return nil, err
	}
	system, err := strconv.ParseInt(strings.Split(cpuStat[1], " ")[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return []int64{user, system}, nil
}

// ConsoleGetFD allocates a console tty from container
func (lxc *Container) ConsoleGetFD(ttynum int) (int, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return -1, err
	}

	lxc.Lock()
	defer lxc.Unlock()

	ret := int(C.lxc_container_console_getfd(lxc.container, C.int(ttynum)))
	if ret < 0 {
		return -1, fmt.Errorf(errAttachFailed, C.GoString(lxc.container.name))
	}
	return ret, nil
}

// Console allocates and runs a console tty from container
func (lxc *Container) Console(ttynum, stdinfd, stdoutfd, stderrfd, escape int) error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	if !bool(C.lxc_container_console(lxc.container, C.int(ttynum), C.int(stdinfd), C.int(stdoutfd), C.int(stderrfd), C.int(escape))) {
		return fmt.Errorf(errAttachFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// AttachRunShell runs a shell inside the container
func (lxc *Container) AttachRunShell() error {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	ret := int(C.lxc_container_attach(lxc.container))
	if ret < 0 {
		return fmt.Errorf(errAttachFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// AttachRunCommand runs user specified command inside the container and waits it
func (lxc *Container) AttachRunCommand(args ...string) error {
	if args == nil {
		return fmt.Errorf(errInsufficientNumberOfArguments)
	}

	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return err
	}

	lxc.Lock()
	defer lxc.Unlock()

	cargs := makeArgs(args)
	defer freeArgs(cargs, len(args))

	ret := int(C.lxc_container_attach_run_wait(lxc.container, cargs))
	if ret < 0 {
		return fmt.Errorf(errAttachFailed, C.GoString(lxc.container.name))
	}
	return nil
}

// Interfaces returns the name of the interfaces from the container
func (lxc *Container) Interfaces() ([]string, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	result := C.lxc_container_get_interfaces(lxc.container)
	if result == nil {
		return nil, fmt.Errorf(errInterfaces, C.GoString(lxc.container.name))
	}
	return convertArgs(result), nil
}

// IPAddress returns the IP address of the given interface
func (lxc *Container) IPAddress(interfaceName string) ([]string, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	cinterface := C.CString(interfaceName)
	defer C.free(unsafe.Pointer(cinterface))

	result := C.lxc_container_get_ips(lxc.container, cinterface, nil, 0)
	if result == nil {
		return nil, fmt.Errorf(errIPAddress, interfaceName, C.GoString(lxc.container.name))
	}
	return convertArgs(result), nil
}

// IPAddresses returns all IP addresses from the container
func (lxc *Container) IPAddresses() ([]string, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	result := C.lxc_container_get_ips(lxc.container, nil, nil, 0)
	if result == nil {
		return nil, fmt.Errorf(errIPAddresses, C.GoString(lxc.container.name))
	}
	return convertArgs(result), nil

}

// IPv4Addresses returns all IPv4 addresses from the container
func (lxc *Container) IPv4Addresses() ([]string, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	cfamily := C.CString("inet")
	defer C.free(unsafe.Pointer(cfamily))

	result := C.lxc_container_get_ips(lxc.container, nil, cfamily, 0)
	if result == nil {
		return nil, fmt.Errorf(errIPv4Addresses, C.GoString(lxc.container.name))
	}
	return convertArgs(result), nil
}

// IPv6Addresses returns all IPv6 addresses from the container
func (lxc *Container) IPv6Addresses() ([]string, error) {
	if err := lxc.ensureDefinedAndRunning(); err != nil {
		return nil, err
	}

	lxc.RLock()
	defer lxc.RUnlock()

	cfamily := C.CString("inet6")
	defer C.free(unsafe.Pointer(cfamily))

	result := C.lxc_container_get_ips(lxc.container, nil, cfamily, 0)
	if result == nil {
		return nil, fmt.Errorf(errIPv6Addresses, C.GoString(lxc.container.name))
	}
	return convertArgs(result), nil
}
