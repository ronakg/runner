package lib

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleCommands tests running simple shell commands, their completion status and output
func TestSimpleCommands(t *testing.T) {
	Debug = true
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
			command:  "echo foo >> /dev/stderr",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "foo\n",
		},
		{
			name:     "redirect to stdout",
			command:  "echo bar >> /dev/stdout",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 0,
			output:   "bar\n",
		},
		{
			name:     "failing command",
			command:  "cat invalid_file",
			nilErr:   true,
			nilJob:   false,
			status:   StatusCompleted,
			exitCode: 1,
			output:   "cat: invalid_file: No such file or directory\n",
		},
		{
			name:    "blank command",
			command: "",
			nilErr:  false,
			nilJob:  true,
		},
		{
			name:     "background command",
			command:  "echo foo && echo bar &",
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
				time.Sleep(time.Second)

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
	Debug = true
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
			time.Sleep(tc.timeout + time.Second)

			// status
			assertStatus(t, j, StatusTimedOut, -1)

			// output
			assertOutput(t, j, tc.output)
		})
	}
}

// TestStop tests stopping a job
func TestStop(t *testing.T) {
	Debug = true
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
			command:          "for i in $(seq 1 10); do echo iteration $i; sleep 1; done",
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
	Debug = true
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
	Debug = true
	testCases := []struct {
		name       string // test case name
		command    string // command to run
		numClients int    // number of clients
		output     string // output after output cancellation
	}{
		{
			name:       "10 clients output cancellation",
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
	Debug = true
	testCases := []struct {
		name        string        // test case name
		command     string        // command to run
		stopWaitDur time.Duration // duration to wait before stopping
		numClients  int           // number of clients
		output      string        // output
	}{
		{
			name:        "10 clients output cancellation",
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
	Debug = true
	testCases := []struct {
		name     string // test case name
		command  string // command to run
		numStops int    // number of times Stop should be called concurrently
	}{
		{
			name:     "10 clients output cancellation",
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

// assertOutput is a convenience function to stream and verify a job's output
func assertOutput(t *testing.T, j Job, expected string) {
	out, cancel, err := j.Output()
	require.Nil(t, err)

	defer cancel()

	output := make([]byte, 0)
	for b := range out {
		output = append(output, b.Bytes...)
	}
	assert.Equal(t, expected, string(output))
}

// assertStatus is a convenience function to verify a job's status
func assertStatus(t *testing.T, j Job, expectedStatus JobStatus, expectedCode int) {
	status, exitCode := j.Status()
	assert.Equal(t, expectedStatus, status)
	assert.Equal(t, expectedCode, exitCode)
}
