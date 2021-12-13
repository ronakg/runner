package integration

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	runnerHome  = "/tmp/runner"
	clientCerts = filepath.Join(runnerHome, "certs/")
	serverCerts = filepath.Join(runnerHome, "certs/server")
	clientBin   = filepath.Join(runnerHome, "bin/client")
	serverBin   = filepath.Join(runnerHome, "bin/server")
)

func TestSimpleCommands(t *testing.T) {
	testCases := []struct {
		desc    string
		client  string
		command string
		timeout int
		output  string
		status  string
	}{
		{
			desc:    "echo command",
			client:  "validclient1",
			command: "echo 123",
			timeout: 0,
			output:  "123\n",
			status:  "COMPLETED (0)",
		},
		{
			desc:    "pipe command",
			client:  "validclient1",
			command: "echo this is a pipe test | grep -o pipe",
			timeout: 0,
			output:  "pipe\n",
			status:  "COMPLETED (0)",
		},
		{
			desc:    "command chain",
			client:  "validclient2",
			command: "echo abc && echo xyz && echo 123 && echo 456",
			timeout: 0,
			output:  "abc\nxyz\n123\n456\n",
			status:  "COMPLETED (0)",
		},
	}
	defer startServer(t)()

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// start
			id, err := startClient(tc.client, tc.command, tc.timeout)
			if err != nil {
				t.Errorf("Failed to start client: %v", err)
			}

			// output
			output, err := getOutput(tc.client, id)
			if err != nil {
				t.Errorf("Failed to fetch output: %v", err)
			}
			assert.Equal(t, tc.output, output)

			// status
			status, err := getStatus(tc.client, id)
			if err != nil {
				t.Errorf("Failed to get status: %v", err)
			}
			assert.Equal(t, tc.status, status)

			// stop
			status, err = stopClient(tc.client, id)
			if err != nil {
				t.Errorf("Failed to get status: %v", err)
			}
			assert.Equal(t, tc.status, status)
		})
	}
}

func TestAuthentication(t *testing.T) {
	testCases := []struct {
		client string
		noErr  bool
	}{
		{
			client: "validclient1",
			noErr:  true,
		},
		{
			client: "validclient2",
			noErr:  true,
		},
		{
			client: "selfsigned",
			noErr:  false,
		},
	}

	// server
	defer startServer(t)()

	for _, tc := range testCases {
		t.Run(tc.client, func(t *testing.T) {
			_, err := startClient(tc.client, "ls -lrt", 0)
			assert.Equal(t, err == nil, tc.noErr)
		})
	}
}

func TestAuthorization(t *testing.T) {
	// server
	defer startServer(t)()

	id1, err := startClient("validclient1", "ls -lrt", 0)
	require.Nil(t, err)
	id2, err := startClient("validclient2", "ls -lrt", 0)
	require.Nil(t, err)

	_, err = getStatus("validclient2", id1)
	require.NotNil(t, err)
	_, err = getStatus("validclient1", id2)
	require.NotNil(t, err)

	_, err = getOutput("validclient2", id1)
	require.NotNil(t, err)
	_, err = getOutput("validclient1", id2)
	require.NotNil(t, err)

	_, err = stopClient("validclient2", id1)
	require.NotNil(t, err)
	_, err = stopClient("validclient1", id2)
	require.NotNil(t, err)
}

func TestConcurrentOutput(t *testing.T) {
	// server
	defer startServer(t)()

	client := "validclient1"
	expectedOutput := "iteration 1\niteration 2\niteration 3\niteration 4\niteration 5\n"
	id, err := startClient(client, "for i in $(seq 1 5); do echo iteration $i; sleep 1; done", 0)
	require.Nil(t, err)

	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			output, err := getOutput(client, id)
			if err != nil {
				t.Errorf("Failed to get output: %v", err)
			}
			assert.Equal(t, expectedOutput, output)

			// status
			status, err := getStatus(client, id)
			if err != nil {
				t.Errorf("Failed to get status: %v", err)
			}
			assert.Equal(t, "COMPLETED (0)", status)

			// stop
			status, err = stopClient(client, id)
			if err != nil {
				t.Errorf("Failed to get status: %v", err)
			}
			assert.Equal(t, "COMPLETED (0)", status)
		}()
	}
	wg.Wait()
}

func startClient(client, command string, timeout int) (id string, err error) {
	clientArgs := []string{"--certs", filepath.Join(clientCerts, client), "start", "--timeout", strconv.Itoa(timeout), command}
	cmd := exec.Command(clientBin, clientArgs...)
	fmt.Printf("Running command: %s\n", cmd)
	output, err := cmd.CombinedOutput()
	fmt.Println(string(output))
	return string(output[:len(output)-1]), err
}

func getOutput(client, id string) (string, error) {
	clientArgs := []string{"--certs", filepath.Join(clientCerts, client), "output", "--id", id}
	cmd := exec.Command(clientBin, clientArgs...)
	fmt.Printf("Running command: %s\n", cmd)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func getStatus(client, id string) (string, error) {
	clientArgs := []string{"--certs", filepath.Join(clientCerts, client), "status", "--id", id}
	cmd := exec.Command(clientBin, clientArgs...)
	fmt.Printf("Running command: %s\n", cmd)
	output, err := cmd.CombinedOutput()
	return string(output[:len(output)-1]), err
}

func stopClient(client, id string) (string, error) {
	clientArgs := []string{"--certs", filepath.Join(clientCerts, client), "stop", "--id", id}
	cmd := exec.Command(clientBin, clientArgs...)
	fmt.Printf("Running command: %s\n", cmd)
	output, err := cmd.CombinedOutput()
	return string(output[:len(output)-1]), err
}

func startServer(t *testing.T) func() {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, serverBin, serverCerts)
	err := cmd.Start()
	stop := func() {
		cancel()
		err := cmd.Wait()
		fmt.Printf("Sever stopped: %v\n", err)
	}
	require.Nil(t, err)
	time.Sleep(time.Second) // give server some time
	return stop
}
