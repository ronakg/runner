package lib

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	Debug = true
	RootFSSource = "/tmp/runner/rootfs"
}

// TestSimpleCommands tests running simple shell commands, their completion status and output
func TestSimpleCommands(t *testing.T) {
	testCases := []struct {
		name     string    // name of the test case
		command  string    // command to be tested
		nilErr   bool      // nil error from StartJob?
		nilJob   bool      // nil job from StartJob?
		status   JobStatus // completion status
		exitCode int       // exit code
		output   string    // output
	}{
		{
			name:     "echo",
			command:  "echo 123",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "123\n",
		},
		{
			name:     "pipe",
			command:  "echo this is a pipe test | grep -o pipe",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "pipe\n",
		},
		{
			name:     "command chain",
			command:  "echo abc && echo xyz && echo 123 && echo 456",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "abc\nxyz\n123\n456\n",
		},
		{
			name:     "redirect to stderr",
			command:  "echo foo >&2",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "foo\n",
		},
		{
			name:     "failing command",
			command:  "cat invalid_file",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 1,
			output:   "cat: can't open 'invalid_file': No such file or directory\n",
		},
		{
			name:    "blank command",
			command: "",
			nilErr:  false,
			nilJob:  true,
		},
		{
			name:     "command chain",
			command:  "echo foo && echo bar",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "foo\nbar\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.Equal(t, tc.nilErr, err == nil)
			require.Equal(t, tc.nilJob, j == nil)

			if j != nil {
				// let the job finish
				j.Wait()

				// id
				assert.NotEmpty(t, j.ID())

				// status
				assertStatus(t, j, tc.status, tc.exitCode)

				// output
				assertOutput(t, j, tc.output)
			}
		})
	}
}

// TestTimeout tests timeout expiration for jobs
func TestTimeout(t *testing.T) {
	testCases := []struct {
		name    string        // test case name
		command string        // command to run
		timeout time.Duration // timeout
		output  string        // output
	}{
		{
			name:    "1s timeout",
			command: "echo 123 && sleep 5 && echo 456",
			timeout: time.Second,
			output:  "123\n",
		},
		{
			name:    "3s timeout",
			command: "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			timeout: 3 * time.Second,
			output:  "iteration 1\niteration 2\niteration 3\n",
		},
		{
			name:    "5s timeout",
			command: "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			timeout: 5 * time.Second,
			output:  "iteration 1\niteration 2\niteration 3\niteration 4\niteration 5\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
				Timeout: tc.timeout,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			// let the job time out
			j.Wait()

			// status
			assertStatus(t, j, StatusTimedOut, -1)

			// output
			assertOutput(t, j, tc.output)
		})
	}
}

// TestStop tests stopping a job
func TestStop(t *testing.T) {
	testCases := []struct {
		name             string        // test case name
		command          string        // command to run
		stopWaitDuration time.Duration // how long to wait before calling Stop()
		output           string        // output
	}{
		{
			name:             "stop immediately",
			command:          "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			stopWaitDuration: 0,
			output:           "",
		},
		{
			name:             "stop after 1s",
			command:          "for i in $(seq 1 10); do echo iteration $i; sleep 2; done",
			stopWaitDuration: time.Second,
			output:           "iteration 1\n",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			// Let the job time out
			time.Sleep(tc.stopWaitDuration)

			// Verify that calling Stop() multiple times is okay
			j.Stop()
			j.Stop()
			j.Stop()

			// status
			assertStatus(t, j, StatusStopped, -1)

			// output
			assertOutput(t, j, tc.output)
		})
	}
}

// TestConcurrentOutput tests streaming output from 1 job to multiple clients
func TestConcurrentOutput(t *testing.T) {
	testCases := []struct {
		name       string // test case name
		command    string // command to run
		numClients int    // number of output clients to run
		output     string // output
	}{
		{
			name:       "50 clients",
			command:    "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			numClients: 50,
			output:     "iteration 1\niteration 2\niteration 3\niteration 4\niteration 5\niteration 6\niteration 7\niteration 8\niteration 9\niteration 10\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			// Start multiple concurrent output clients
			wg := sync.WaitGroup{}
			wg.Add(tc.numClients)

			for i := 0; i < tc.numClients; i++ {
				go func(i int) {
					defer wg.Done()

					// Wait a random number of seconds for each client
					// This is to test that output streaming always starts from the beginning of the
					// job's output
					<-time.After(time.Duration(rand.Intn(10)) * time.Second)

					t.Logf("Starting output client %d", i+1)
					assertOutput(t, j, tc.output)
				}(i)
			}
			wg.Wait()

			// one more client after the job is complete
			t.Logf("Starting last output client")
			assertOutput(t, j, tc.output)

			// status
			assertStatus(t, j, StatusCompleted, 0)
		})
	}
}

// TestOutputCancellation tests cancellation of output while multiple clients are consuming from out channels
func TestOutputCancellation(t *testing.T) {
	testCases := []struct {
		name       string // test case name
		command    string // command to run
		numClients int    // number of clients
		output     string // output after output cancellation
	}{
		{
			name:       "10 clients",
			command:    "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			numClients: 10,
			output:     "iteration 1\niteration 2\niteration 3\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			// Start multiple concurrent output clients
			wg := sync.WaitGroup{}
			wg.Add(tc.numClients)

			for i := 0; i < tc.numClients; i++ {
				go func(i int) {
					defer wg.Done()

					t.Logf("Starting output client %d", i+1)
					out, cancel, err := j.Output()
					require.Nil(t, err)

					// Start streaming output in this goroutine
					output := make([]byte, 0)
					for b := range out {
						output = append(output, b.Bytes...)
						// cancel output after we've received 3 iterations
						if string(output) == tc.output {
							// cancel multiple times to verify that subsequent cancellations are no-op
							cancel()
							cancel()
							cancel()
						}
					}
					assert.Equal(t, tc.output, string(output))
				}(i)
			}
			wg.Wait()
		})
	}
}

// TestStopDuringOutput tests stopping a job while multiple concurrent clients are consuming output
func TestStopDuringOutput(t *testing.T) {
	testCases := []struct {
		name        string        // test case name
		command     string        // command to run
		stopWaitDur time.Duration // duration to wait before stopping
		numClients  int           // number of clients
		output      string        // output
	}{
		{
			name:        "10 clients",
			command:     "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
			stopWaitDur: 3 * time.Second,
			numClients:  10,
			output:      "iteration 1\niteration 2\niteration 3\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			// Start multiple concurrent clients
			wg := sync.WaitGroup{}
			wg.Add(tc.numClients)
			for i := 0; i < tc.numClients; i++ {
				go func(i int) {
					defer wg.Done()

					t.Logf("Starting output client %d", i+1)
					assertOutput(t, j, tc.output)
				}(i)
			}
			<-time.After(tc.stopWaitDur)
			j.Stop()

			wg.Wait()

			// Status
			assertStatus(t, j, StatusStopped, -1)
		})
	}
}

// TestConcurrentStops tests the scenario where Stop() is called multiple times from concurrent goroutines
func TestConcurrentStops(t *testing.T) {
	testCases := []struct {
		name     string // test case name
		command  string // command to run
		numStops int    // number of times Stop should be called concurrently
	}{
		{
			name:     "10 clients",
			numStops: 10,
			command:  "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)

			wg := sync.WaitGroup{}
			for i := 0; i < tc.numStops; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					j.Stop()
				}()
			}
			wg.Wait()

			// Status
			assertStatus(t, j, StatusStopped, -1)
		})
	}

}

// TestNoOutputCancellation tests cancellation of output when the job doesn't generate any output
func TestNoOutputCancellation(t *testing.T) {
	testCases := []struct {
		name           string        // test case name
		command        string        // command to run
		cancelDuration time.Duration // duration to wait before stopping
		numClients     int           // number of clients
		output         string        // output
	}{
		{
			name:           "10 clients",
			command:        "sleep 3600", // go test should timeout in case of failure
			cancelDuration: 3 * time.Second,
			numClients:     10,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := JobConfig{
				Command: tc.command,
			}
			j, err := StartJob(c)
			require.NotNil(t, j)
			require.Nil(t, err)
			defer j.Stop()

			// Start multiple concurrent clients
			wg := sync.WaitGroup{}
			wg.Add(tc.numClients)
			for i := 0; i < tc.numClients; i++ {
				go func(i int) {
					defer wg.Done()

					t.Logf("Starting output client %d", i+1)
					out, cancel, err := j.Output()
					require.Nil(t, err)

					// start a goroutine that'll cancel the output streaming
					go func(cancel func()) {
						time.Sleep(tc.cancelDuration)
						cancel()
					}(cancel)

					// Start streaming output in this goroutine
					for b := range out {
						t.Errorf("Unexpected output: %s", b)
					}
				}(i)
			}
			wg.Wait()

			// Status
			assertStatus(t, j, StatusRunning, -1)
		})
	}
}

// TestPIDIsolation tests PID isolation for the job by running concurrent jobs that run "ps -ef" and
// making sure that output is small and doesn't contain processes from the host
func TestPIDIsolation(t *testing.T) {
	Debug = true
	testCases := []struct {
		name    string // test case name
		command string // command to run
		numJobs int    // number of clients
	}{
		{
			name:    "10 clients",
			command: "ps -ef | wc -l",
			numJobs: 10,
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wg := sync.WaitGroup{}
			wg.Add(tc.numJobs)
			for i := 0; i < tc.numJobs; i++ {
				go func() {
					defer wg.Done()

					c := JobConfig{
						Command: tc.command,
					}
					j, err := StartJob(c)
					require.NotNil(t, j)
					require.Nil(t, err)

					output := getOutput(t, j)
					num, err := strconv.Atoi(output[:len(output)-1])
					assert.Nil(t, err)
					assert.LessOrEqual(t, num, 5)
					j.Wait()

					// Status
					assertStatus(t, j, StatusCompleted, 0)
				}()
			}
			wg.Wait()
		})
	}
}

// TestMountIsolation tests mount isolation for the job
// Run concurrent clients that output text to same file and make sure that the file is contained
// within the job's root filesystem
func TestMountIsolation(t *testing.T) {
	Debug = true
	testCases := []struct {
		name    string // test case name
		command string // command to run
		numJobs int    // number of clients
		output  string // output
	}{
		{
			name:    "10 clients",
			command: "echo test for job %d >> /test.log && cat /test.log",
			numJobs: 10,
			output:  "test for job %d\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wg := sync.WaitGroup{}
			wg.Add(tc.numJobs)
			for i := 0; i < tc.numJobs; i++ {
				go func(i int) {
					defer wg.Done()

					c := JobConfig{
						Command: fmt.Sprintf(tc.command, i),
					}
					j, err := StartJob(c)
					require.NotNil(t, j)
					require.Nil(t, err)
					j.Wait()
					assertOutput(t, j, fmt.Sprintf(tc.output, i))

					// Status
					assertStatus(t, j, StatusCompleted, 0)
				}(i)
			}
			wg.Wait()
		})
	}
}

// TestNetworkIsolation tests network isolation for a job
// Run multiple concurrent clients that change the hostname to a specific value and verify that the
// server's hostname is not affected
func TestNetworkIsolation(t *testing.T) {
	Debug = true
	testCases := []struct {
		name    string // test case name
		command string // command to run
		numJobs int    // number of clients
		output  string
	}{
		{
			name:    "10 clients",
			command: "ip addr show | grep inet | wc -l",
			numJobs: 10,
			output:  "0\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wg := sync.WaitGroup{}
			wg.Add(tc.numJobs)
			for i := 0; i < tc.numJobs; i++ {
				go func() {
					defer wg.Done()

					c := JobConfig{
						Command: tc.command,
					}
					j, err := StartJob(c)
					require.NotNil(t, j)
					require.Nil(t, err)

					assertOutput(t, j, tc.output)
					j.Wait()

					// Status
					assertStatus(t, j, StatusCompleted, 0)
				}()
			}
			wg.Wait()
		})
	}
}

func TestHostNameIsolation(t *testing.T) {
	Debug = true
	testCases := []struct {
		name    string // test case name
		command string // command to run
		numJobs int    // number of clients
		output  string // output
	}{
		{
			name:    "10 clients",
			command: "hostname job%d && hostname",
			numJobs: 10,
			output:  "job%d\n",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wg := sync.WaitGroup{}
			wg.Add(tc.numJobs)
			for i := 0; i < tc.numJobs; i++ {
				go func(i int) {
					defer wg.Done()

					c := JobConfig{
						Command: fmt.Sprintf(tc.command, i),
					}
					j, err := StartJob(c)
					require.NotNil(t, j)
					require.Nil(t, err)
					j.Wait()
					output := getOutput(t, j)
					assertOutput(t, j, fmt.Sprintf(tc.output, i))

					// Status
					assertStatus(t, j, StatusCompleted, 0)

					serverHostname, err := os.Hostname()
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					jobHostname := output[:len(output)-1]
					t.Logf("Hostnames - Server: %s, job: %s", serverHostname, jobHostname)
					assert.NotEqual(t, serverHostname, jobHostname)
				}(i)
			}
			wg.Wait()
		})
	}
}

// assertOutput is a convenience function to verify a job's output
func assertOutput(t *testing.T, j Job, expected string) {
	assert.Equal(t, expected, getOutput(t, j))
}

// getOutput is a convenience function to return a job's output
func getOutput(t *testing.T, j Job) string {
	out, cancel, err := j.Output()
	require.Nil(t, err)
	defer cancel()

	output := make([]byte, 0)
	for b := range out {
		output = append(output, b.Bytes...)
	}
	return string(output)
}

// assertStatus is a convenience function to verify a job's status
func assertStatus(t *testing.T, j Job, expectedStatus JobStatus, expectedCode int) {
	status, exitCode := j.Status()
	assert.Equal(t, expectedStatus, status)
	if status != StatusRunning {
		// exitCode is undefined when status is StatusRunning
		assert.Equal(t, expectedCode, exitCode)
	}
}
