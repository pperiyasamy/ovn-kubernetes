//go:build linux
// +build linux

package ovspinning

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/unix"

	"k8s.io/klog/v2"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
)

// These variables are meant to be used in unit tests
var tickDuration time.Duration = 1 * time.Second
var getOvsVSwitchdPIDFn func() (string, error) = util.GetOvsVSwitchdPID
var getOvsDBServerPIDFn func() (string, error) = util.GetOvsDBServerPID
var featureEnablerFile string = "/etc/openvswitch/enable_dynamic_cpu_affinity"

// Run monitors OVS daemon's processes (ovs-vswitchd and ovsdb-server) and sets their CPU affinity
// masks to that of the current process.
// This feature is enabled by the presence of a non-empty file in the path `/etc/openvswitch/enable_dynamic_cpu_affinity`
func Run(stopCh <-chan struct{}) {

	// The file must be present at startup to enable the feature
	isFeatureEnabled, err := isFileNotEmpty(featureEnablerFile)
	if err != nil {
		klog.Warningf("Can't start OVS CPU affinity pinning: %v", err)
		return
	}

	if !isFeatureEnabled {
		klog.Info("OVS CPU affinity pinning disabled")
		return
	}

	klog.Infof("Starting OVS daemon CPU pinning")
	defer klog.Infof("Stopping OVS daemon CPU pinning")

	var fsnotifyEvents chan fsnotify.Event
	var fsnotifyErrors chan error

	// Watch the parent folder, as it's the only way to get events when the file is deleted and recreated.
	fileWatcher, err := createFileWatcherFor(filepath.Dir(featureEnablerFile))
	if err != nil {
		klog.Warningf("Can't create a watcher for %s. Pinning will not stop by deleting it: %v", featureEnablerFile, err)
		fsnotifyEvents = make(chan fsnotify.Event)
		fsnotifyErrors = make(chan error)
	} else {
		fsnotifyEvents = fileWatcher.Events
		fsnotifyErrors = fileWatcher.Errors
		defer fileWatcher.Close()
	}

	ticker := time.NewTicker(tickDuration)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-fsnotifyEvents:
			if !ok {
				continue
			}

			// Since we are watching the entire folder, skip all the events not related to the enabler file
			if event.Name != featureEnablerFile {
				continue
			}

			isFeatureEnabled, err = isFileNotEmpty(featureEnablerFile)
			if err != nil {
				klog.Warningf("Error while reading [%s]: %v", featureEnablerFile, err)
				continue
			}

			if isFeatureEnabled {
				klog.Infof("OVS daemon CPU pinning feature enabled")
			} else {
				klog.Infof("OVS daemon CPU pinning feature NOT enabled")
			}

		case err, ok := <-fsnotifyErrors:
			if ok {
				klog.Errorf("Error watching for file [%s] changes: %s", featureEnablerFile, err)
			}

		case <-stopCh:
			return

		case <-ticker.C:
			if !isFeatureEnabled {
				continue
			}

			err := setOvsVSwitchdCPUAffinity()
			if err != nil {
				klog.Warningf("Error while aligning ovs-vswitchd CPUs to current process: %v", err)
			}

			err = setOvsDBServerCPUAffinity()
			if err != nil {
				klog.Warningf("Error while aligning ovsdb-server CPUs to current process: %v", err)
			}
		}
	}
}

func createFileWatcherFor(filename string) (*fsnotify.Watcher, error) {
	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	err = fileWatcher.Add(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to watch [%s] file: %w", filename, err)
	}

	return fileWatcher, nil
}

func isFileNotEmpty(filename string) (bool, error) {
	f, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("can't get file information [%s]: %w", filename, err)
	}

	// get the size
	return f.Size() > 0, nil
}

func setOvsVSwitchdCPUAffinity() error {

	ovsVSwitchdPID, err := getOvsVSwitchdPIDFn()
	if err != nil {
		return fmt.Errorf("can't retrieve ovs-vswitchd PID: %w", err)
	}

	klog.V(5).Infof("Managing ovs-vswitchd[%s] daemon CPU affinity", ovsVSwitchdPID)
	return setProcessCPUAffinity(ovsVSwitchdPID)
}

func setOvsDBServerCPUAffinity() error {

	ovsDBserverPID, err := getOvsDBServerPIDFn()
	if err != nil {
		return fmt.Errorf("can't retrieve ovsdb-server PID: %w", err)
	}

	klog.V(5).Infof("Managing ovsdb-server[%s] daemon CPU affinity", ovsDBserverPID)
	return setProcessCPUAffinity(ovsDBserverPID)
}

// setProcessCPUAffinity sets the CPU affinity of the given process to the same affinity as the current process
func setProcessCPUAffinity(targetPIDStr string) error {

	targetPID, err := strconv.Atoi(targetPIDStr)
	if err != nil {
		return fmt.Errorf("can't convert PID[%s] to integer: %w", targetPIDStr, err)
	}

	var currentProcessCPUs unix.CPUSet
	err = unix.SchedGetaffinity(os.Getpid(), &currentProcessCPUs)
	if err != nil {
		return fmt.Errorf("can't get own CPU affinity")
	}

	var targetProcessCPUs unix.CPUSet
	err = unix.SchedGetaffinity(targetPID, &targetProcessCPUs)
	if err != nil {
		return fmt.Errorf("can't get process (PID:%d) CPU affinity: %w", targetPID, err)
	}

	if currentProcessCPUs == targetProcessCPUs {
		klog.V(5).Infof("Process[%d] CPU affinity already match current process's affinity %s", targetPID, printCPUSet(currentProcessCPUs))
		return nil
	}

	taskIDs, err := getThreadsOfProcess(targetPID)
	if err != nil {
		return fmt.Errorf("can't get tasks of PID(%d):%w", targetPID, err)
	}

	klog.Infof("Setting CPU affinity of PID(%d) (ntasks=%d) to %s, was %s", targetPID, len(taskIDs), printCPUSet(currentProcessCPUs), printCPUSet(targetProcessCPUs))
	for _, taskID := range taskIDs {
		err = unix.SchedSetaffinity(taskID, &currentProcessCPUs)
		if err != nil {
			// The task may have been stopped, don't break the loop and continue setting CPU affinity on other tasks.
			klog.Warningf("Error while setting CPU affinity of task(%d) PID(%d) to %s: %v", taskID, targetPID, printCPUSet(currentProcessCPUs), err)
		}
	}

	return nil
}

// printCPUSet takes a unix.CPUSet and returns a string representation in canonical linux CPU list format.
// e.g. 0-5,8,10,12-3
//
// See http://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS
func printCPUSet(cpus unix.CPUSet) string {

	type rng struct {
		start int
		end   int
	}

	// Start with a fake range to avoid going out of range while looping
	ranges := []rng{{-2, -2}}

	// There is no public API to know the length of unix.CPUSet, so this counter is the
	// stopping condition for the loop
	remainingSetsCpus := cpus.Count()

	for i := 0; remainingSetsCpus > 0; i++ {
		if !cpus.IsSet(i) {
			continue
		}

		remainingSetsCpus--

		lastRange := ranges[len(ranges)-1]
		if lastRange.end == i-1 {
			ranges[len(ranges)-1].end++
		} else {
			ranges = append(ranges, rng{start: i, end: i})
		}
	}

	var result bytes.Buffer
	// discard the fake range with [1:]
	for _, r := range ranges[1:] {
		if r.start == r.end {
			result.WriteString(strconv.Itoa(r.start))
		} else {
			result.WriteString(fmt.Sprintf("%d-%d", r.start, r.end))
		}
		result.WriteString(",")
	}
	return strings.TrimRight(result.String(), ",")
}

// getThreadsOfProcess returns the list of thread IDs of the given process
func getThreadsOfProcess(pid int) ([]int, error) {
	taskFolders, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return nil, fmt.Errorf("unable to find %d tasks: %v", pid, err)
	}

	ret := []int{}
	for _, taskFolder := range taskFolders {
		taskID, err := strconv.Atoi(taskFolder.Name())
		if err != nil {
			return nil, fmt.Errorf("unable to get task ID of %d: %s, %v", pid, taskFolder.Name(), err)
		}

		ret = append(ret, taskID)
	}

	return ret, nil
}
