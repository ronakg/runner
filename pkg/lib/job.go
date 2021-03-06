// package lib is a library that provides all the job management utilities for Runner
package lib

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docker/docker/pkg/reexec"
	"github.com/fsnotify/fsnotify"
	dirCopy "github.com/otiai10/copy"
)

// TODO: These are global exported variables for now. They should be part of some sort library
// configuration that can be initialized by the user.
var (
	RunnerHome   = "/tmp/runner"
	RootFSSource string // path to the new root file system for jobs
)

func init() {
	reexec.Register("reExecHandler", reExecHandler)

	// if reexec handler is already invoked, then exit to avoid reexec'ing forever
	if reexec.Init() {
		os.Exit(0)
	}

	// setup RunnerHome if it doesn't exist already
	err := os.MkdirAll(RunnerHome, 0755)
	if err != nil {
		log.Fatalf("Failed to initialize library: %v", err)
	}
}

// ResProfile is the name of the resource profile that should be applied to the job
type ResProfile string

const (
	ResProfileDefault ResProfile = "default"
	outputBufSize     int        = 1024
)

// Output represents a few bytes of output generated by a job
type Output struct {
	Bytes []byte
}

// JobConfig represents the configuration required to start a job
type JobConfig struct {
	Command string        // Command including arguments to run as a job
	Timeout time.Duration // Timeout determines how long a job is allowed to run
	Profile ResProfile    // Profile determines the resource profile that should be applied to a job
}

// Job is the interface that wraps all the functions of a job
type Job interface {
	// ID returns the job identifier
	ID() string

	// Stop stops a running job
	Stop()

	// Status returns the status and exit code of the job
	Status() (status JobStatus, exitCode int)

	// Output returns an out channel from which the output of a job can be consumed. The cancel
	// function can be used to stop streaming output from the job. Once cancel function is invoked,
	// the out channel is closed
	Output() (out <-chan *Output, cancel func(), err error)

	// Wait waits for the job to finish
	Wait()
}

// job is the concrete implementation of Job
type job struct {
	config           JobConfig
	id               string
	outFile          string        // Path to the file where output is stored
	status           safeJobStatus // Status of the job
	exitCode         int32         // Exit code of the job
	cmd              *exec.Cmd
	outputWriterDone chan struct{}  // channel to notify that outputWriter goroutine is done
	stopOnce         sync.Once      // Used to make sure Stop is executed only once
	wg               sync.WaitGroup // To make sure all goroutines come to stop
	rootFSPath       string         // path to the root filesystem for the job
}

func (j *job) String() string {
	return fmt.Sprintf("Job[id='%s', command='%s', status='%s']",
		j.id, j.config.Command, j.status.Get())
}

// StartJob starts a new job according to supplied JobConfig
func StartJob(config JobConfig) (Job, error) {
	if config.Command == "" {
		return nil, errors.New("config.Path is empty")
	}

	id, err := generateJobID()
	if err != nil {
		return nil, err
	}

	j := &job{
		id:               id,
		config:           config,
		outFile:          filepath.Join(RunnerHome, id, "output.log"),
		status:           safeJobStatus{value: StatusCreated},
		exitCode:         -1,
		outputWriterDone: make(chan struct{}),
		rootFSPath:       filepath.Join(RunnerHome, id, "rootfs"),
	}
	debugLog("%s created", j)

	// Set up root filesystem for the job
	// <RunnerHome>/<job_id>/rootfs
	err = j.createRootFSTree()
	if err != nil {
		debugLog("Failed to create root filesystem for %s: %v", j, err)
		return nil, err
	}

	j.setupReExecCommand()

	if err := j.startOutputWriter(); err != nil {
		return nil, err
	}

	debugLog("Starting %s", j)
	err = j.cmd.Start()
	if err != nil {
		debugLog("Failed to start %s: %v", j, err)
		return nil, err
	}
	j.status.Set(StatusRunning)

	// Start waiter
	j.wg.Add(1)
	go j.waiter()

	return j, nil
}

// ID returns the job identifier
func (j *job) ID() string {
	return j.id
}

// kill kills all the processes spawned by the job including any child processes
func (j *job) kill(status JobStatus) {
	// Make sure that we only kill the process once
	j.stopOnce.Do(func() {
		if j.status.Get() == StatusRunning {
			// Just cancelling the context doesn't stop all child processes
			// Passing a negative PID to the syscall sends a SIGKILL signal to all the child processes
			err := syscall.Kill(-j.cmd.Process.Pid, syscall.SIGKILL)
			if err != nil {
				debugLog("Failed to stop the job: %v", err)
			}
			j.status.Set(status)
		}
	})
}

// Stop stops the job and waits for all the goroutines to finish processing
func (j *job) Stop() {
	debugLog("Stopping %s", j)
	j.kill(StatusStopped)
	j.wg.Wait()
}

// Status returns the status of the job and the exit code.
// Exit code is undefined if the status is StatusRunning.
func (j *job) Status() (status JobStatus, exitCode int) {
	return j.status.Get(), int(atomic.LoadInt32(&j.exitCode))
}

// Output returns an out channel from which the output of a job can be consumed. The cancel
// function can be used to stop streaming output from the job. Once cancel function is invoked,
// the out channel is closed.
func (j *job) Output() (out <-chan *Output, cancel func(), err error) {
	// cancelOnce is used to make sure that cancel() is executed only once
	cancelOnce := sync.Once{}

	// Channel to signal the cancellation by the caller to the goroutine writing to out channel
	canceled := make(chan struct{})

	// cancel func that's returned to the user
	cancel = func() {
		cancelOnce.Do(func() {
			close(canceled)
		})
	}

	// outChan is the output channel that's returned to the caller
	outChan := make(chan *Output)

	// Set up a file watcher to monitor changes to j.outFile
	watcher, err := j.outputWatcher()
	if err != nil {
		return nil, nil, err
	}

	f, err := os.Open(j.outFile)
	if err != nil {
		return nil, nil, err
	}

	// goroutine to read the j.outFile and send data to the out channel
	go func() {
		defer close(outChan)
		defer cancel()

		defer func() {
			if err := f.Close(); err != nil {
				debugLog("Failed to close %s: %v", j.outFile, err)
			}
		}()
		defer func() {
			if err := watcher.Close(); err != nil {
				debugLog("Failed to close watcher for %s: %v", j.outFile, err)
			}
		}()

		debugLog("Starting output for %s", j)
		readOnceMore := true
		buf := make([]byte, outputBufSize)
		for {
			n, err := f.Read(buf)

			// n can be positive even in case of an error
			// Send the read data to out channel
			if n > 0 {
				o := &Output{
					Bytes: make([]byte, n),
				}
				copy(o.Bytes, buf)
				outChan <- o
			}

			if err != nil {
				if !errors.Is(err, io.EOF) {
					debugLog("Failed to read from file %s: %v", j.outFile, err)
					return
				}
			} else {
				continue
			}

			// We've read everything from the j.outFile. Now wait till more output is appended to
			// the file to restart read
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					debugLog("Watcher events channel shut down, exiting")
					return
				}
			case err, ok := <-watcher.Errors:
				if !ok || err != nil {
					debugLog("shutting down, error: %v", err)
					return
				}
			case <-canceled:
				// output streaming canceled by the caller
				debugLog("Stopping output streaming for %s", j)
				return
			case <-j.outputWriterDone:
				// sometimes this event is received before we get the watcher notification. Read the
				// outFile one more time to make sure we read everything.
				if readOnceMore {
					readOnceMore = false
					continue
				}
				return
			}
		}
	}()

	return outChan, cancel, nil
}

// Wait waits for the job to complete
func (j *job) Wait() {
	j.wg.Wait()
}

// waiter is a goroutine that waits for the job to complete and perform cleanup for the job
func (j *job) waiter() {
	defer j.wg.Done()

	debugLog("Starting waiter for %s", j)

	if j.config.Timeout > 0 {
		select {
		case <-time.After(j.config.Timeout):
			// kill the job if the timeout expired
			j.kill(StatusTimedOut)
		case <-j.outputWriterDone:
			// outputWriter finished, which means that the job ran to its completion
		}
	}
	// If the job was stopped due to timeout expiration, we still need to make sure that
	// outputWriter finished writing output to j.outFile
	<-j.outputWriterDone

	// Wait for exec.Cmd to handle process completion
	err := j.cmd.Wait()
	if err != nil {
		debugLog("%s completed with error: %v, code: %d", j, err, j.cmd.ProcessState.ExitCode())
	} else {
		debugLog("%s completed successfully", j)
	}

	j.status.UpdateIf(StatusRunning, StatusCompleted)
	atomic.StoreInt32(&j.exitCode, int32(j.cmd.ProcessState.ExitCode()))

	// Clean up the root fs tree created for the job
	err = j.deleteRootFSTree()
	if err != nil {
		debugLog("Failed to delete root filesystem for %s: %v", j, err)
	}
}

// outputWriter reads data from the mr and writes the same to f
func (j *job) outputWriter(mr io.Reader, f *os.File) {
	defer j.wg.Done()

	// Close outputWriterDone to signal completion of outputWriterDone
	defer close(j.outputWriterDone)
	defer func() {
		if err := f.Close(); err != nil {
			debugLog("Failed to close %s: %v", j.outFile, err)
		}
	}()

	debugLog("Starting outputWriter for %s", j)

	_, err := io.Copy(f, mr)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			debugLog("Failed to read stdout or stderr: %v", err)
		}
	}

	debugLog("outputWriter done for %s", j)
}

// outputWatcher creates a Watcher for the outFile and returns the same
func (j *job) outputWatcher() (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	err = watcher.Add(j.outFile)
	if err != nil {
		return nil, err
	}

	return watcher, nil
}

// generateJobID generates a 12 byte long random ID
func generateJobID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (j *job) setupReExecCommand() {
	// reexec self to setup root filesystem and cgroups
	j.cmd = reexec.Command("reExecHandler", j.rootFSPath, string(j.config.Profile), j.config.Command)

	// Make sure that child processes spawned from the Job belong to same process group
	// This is to make sure that we can stop all the child processes as well in Stop()
	j.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Cloneflags: syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
		// root user inside the job
		Credential: &syscall.Credential{
			Uid: 0,
			Gid: 0,
		},
	}
}

func (j *job) startOutputWriter() error {
	// Set up a TeeReader that reads from stdout and stderr of the job and write the same to outFile
	so, err := j.cmd.StdoutPipe()
	if err != nil {
		debugLog("Failed to capture stdout: %v", err)
		return err
	}
	se, err := j.cmd.StderrPipe()
	if err != nil {
		debugLog("Failed to capture stderr: %v", err)
		return err
	}
	f, err := os.Create(j.outFile)
	if err != nil {
		debugLog("Failed to open output file: %v", err)
		return err
	}

	// Start outputWriter
	j.wg.Add(1)
	go j.outputWriter(io.MultiReader(so, se), f)
	return nil
}

// reExecHandler runs the user's command in a shell
func reExecHandler() {
	rootFSPath := os.Args[1]
	profile := os.Args[2]
	command := os.Args[3]

	debugLog("Spawning command %s with profile %s and rootfs %s", command, profile, rootFSPath)

	if err := rootFSSetup(rootFSPath); err != nil {
		fmt.Printf("failed to set up root fs for %s: %v\n", rootFSPath, err)
		os.Exit(1)
	}

	// TODO: Set up cgroups according to the profile

	cmd := exec.Command("/bin/sh", []string{"-c", command}...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Exit(cmd.ProcessState.ExitCode())
	}
}

func (j *job) createRootFSTree() error {
	debugLog("Creating root filesystem tree for %s", j)
	return dirCopy.Copy(RootFSSource, j.rootFSPath)
}

func (j *job) deleteRootFSTree() error {
	debugLog("Deleting root filesystem tree for %s", j)
	return os.RemoveAll(j.rootFSPath)
}
