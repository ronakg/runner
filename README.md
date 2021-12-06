# Runner

Runner is a tool to easily run and manage jobs on a Linux machine. This document describes the
functional and design aspects of Runner.

## Requirements

### Library

* [x] A method to perform the following operations on a job
    - [x] Start
    - [x] Stop
    - [x] Query status
    - [x] Stream output
* [x] Output of a job should always be from the beginning of the job
* [x] Multiple clients should be able to concurrently retrieve output of a running job
* [ ] Resource control per job using cgroups
    - [ ] CPU
    - [ ] Memory
    - [ ] Disk IO
* [x] Resource isolation for using PID, mount and networking namespaces

### Server

* [x] gRPC API to perform the following operations on a job
    - [x] Start
    - [x] Stop
    - [x] Query status
    - [x] Stream output
* [x] mTLS must be used for authentication between the client(s) and the server
    - [x] A strong set of cipher suits must be used
* [x] A simple authorization scheme is enough for the scope of this project

### Client

* [x] One or more clients should be able to perform the following operations on a job concurrently
    - [x] Start
    - [x] Stop
    - [x] Query status
    - [x] Stream output

### Misc

* [x] Build script to perform the following operations
    - [x] Build client and server binaries for Linux platform
    - [x] Run tests on the entire codebase
    - [x] Generate code coverage report


## Design

The following diagram shows the major components of Runner and their interaction surfaces. Each
component and their interaction with other components are explained in more detail in the following
sections.

![Component Diagram](resources/components.png)

### Library

The library implements all the jobs management interfaces for the server. The server uses the library to
start, stop jobs as well as check the status of and stream output from the existing jobs.

#### Job Management

A job has the following attributes that are provided by the user of the library:
  - `command`: The command (including its arguments) that needs to be executed as a job
  - `timeout`: Timeout duration in seconds to wait for the job to complete. If the job doesn't
  complete within the specified timeout, the job will be stopped by the library. A timeout of 0
  seconds is interpreted as no timeout.
  - `profile`: The resource profile to be applied to the job. Only 1 profile is supported - `default`.

The following attributes are maintained by the library for each job:
  - `id`: The unique identifier of the job. It is generated automatically by the library upon job
  - `status`: The status of the job.
  - `exitCode`: The exit code of the job.
  - `outFile`: The path to the file where output of the job is written to. File path is of the
  format `/tmp/runner/<job_id>.out`.

##### Public interfaces

The following public interfaces are exposed by the library package.

```go
// JobConfig represents the input configuration for a job
type JobConfig struct {
  Command string
  Timeout time.Duration
  Profile string
}

type JobStatus int
const (
  // StatusRunning is returned when the job is still running
  StatusRunning Status = iota
  // StatusCompleted is returned when the job runs to its completion itself
  StatusCompleted
  // StatusStopped is returned when the job is stopped by the user
  StatusStopped
  // StatusTimedOut is returned when the timeout expires and the job is killed
  StatusTimedOut
)

// Output defines the structure for output bytes from a job. Using a struct here provides
// flexibility to include metadata about the line in the future while keeping the package backward
// compatible.  e.g. timestamp or where the output was captured from - stdout vs stderr.
type Output struct {
  Bytes []byte
}

// Job represents the interface containing operations that can be performed on the job
type Job interface {
  // ID returns the job id
  ID() string

  // Stop stops a running job
  Stop()

  // Status returns current JobStatus and an exit code in case the job has terminated
  Status() (JobStatus, int)

  // Output returns an out channel from which the output of a job can be consumed. The cancel
  // function can be used to stop streaming output from the job. Once cancel function is invoked,
  // the out channel is closed.
  Output() (out <-chan *Output, cancel func(), err error)

  // Wait waits for the job to complete. Can be used to synchronously block till the job is
  // finished.
  Wait()
}

// StartJob creates and starts a new job according to supplied JobConfig
func StartJob(c JobConfig) (Job, error) {}

// job implements the Job interface
type job struct {
}
```

Some important considerations for the library:
  - State of all the jobs are only maintained in-memory. This information is not persisted anywhere.
  - Library allows concurrent access to the jobs. Multiple goroutines may perform operations
  simultaneously on the job.
  - A job that has been stopped cannot be started again. A new job has to be created with same input
  parameters for rerun of the job.
  - `Output` returns combined output from `stdout` and `stderr` of the job.

#### Job Execution

- Start: `reexec` is used to spawn the binary of the calling process again in a child process.
  During the reexec phase, the library sets up the root filesystem and the control groups required for
  isolation and resource limitation for the job. Once the setup is complete, user's command is
  executed in its own child process in a controlled environment. The following diagram depicts the
  process hierarchy.

  - `Start` starts 2 goroutines for job management. A goroutine to capture `stdout` and `stderr` of
  the child process and redirect the output to the `outFile` of the job. And another goroutine that
  monitors the job for completion as well as enforces the timeout supplied by the user.

```
+---------+
| server  |
|    |    |     +------------+
| library | --> |   server   |
+---------+     |     |      |
                |  library   |     +---------+
                |   reexec   | --> | command |
                +------------+     +---------+
```

- Stop: When user invokes `Stop` for a job, the child process is killed using the Linux system call
  Kill.

#### Job Resource Limitation 

Each job created using the library is resource limited using cgroups. The library combines all the
resource control limitations of a job into a profile that defines what limits should be applied for
cpu, memory and disk IO for the job. At the moment, only 1 profile called `default` is supported by
the library.

A new cgroup is created for each job. Naming convention for the cgroup for a job is
`runner-<job_id>`.

Example cgroup subsystem setup for a job with id `1234` -
  - `/sys/fs/cgroup/cpu/runner-1234/`
  - `/sys/fs/cgroup/memory/runner-1234/`
  - `/sys/fs/cgroup/blkio/runner-1234/`

#### Job Resource Isolation

Library isolates each job by creating a separate namespace for PID, mount and network. This is
achieved by setting `CLONE_NEWPID`, `CLONE_NEWNS` and `CLONE_NEWNET` clone flags during the job
creation. A new root filesystem is created for the job to make sure that it runs in a completely
isolated environment.

### Server

The server component utilizes the library to provide job management services to the client over a
gRPC channel. Refer to the [protobuf specification](proto/runner.proto) for more details on the gRPC
service and the messages.

### Client

The client component invokes the RPC methods exposed by the server's gRPC service.

The following actions are available to the user of the client.

#### `start`

The `start` action is used to start a new job. The following parameters are required for the `start`
action.

  - `command`: Command that needs to be executed including its arguments.
  - `timeout`: (Optional) Timeout in seconds. Defaults to 0 when not provided.
  - `profile`: (Optional) Resource profile. `default` profile is applied when not provided.

If the job is started successfully by the server, a job id is returned to the client in response.
This job id is required to perform subsequent operations on the job.

#### `stop`

The `stop` action is used to stop a job. It takes only a single parameter called `job_id`.

#### `status`

The `status` action is used to fetch status of a previously created job. It takes only a single
parameter called `job_id`. Status of a job can be `Unknown`, `Running`, `Completed`, `Stopped` or
`Killed`. If the job was stopped because of the timeout expiry, it's status is considered `Killed`.

#### `output`

The `output` action is used to fetch the output generated by the job. It takes only a single
parameter called `job_id`. The job output is streamed from the server and printed on the console
line by line. Multiple clients may stream the output of the same job simultaneously. Output always
starts from the beginning.

### Authentication

The client and the server use mTLS to securely communicate with each other. A private certificate
authority (CA) is used to generate the Root CA, which is used to sign the certificates for the
clients and the server. The server and the clients verify each other's certificates using the common
Root CA. This way, new client certificates can be generated without modifying the server.

Parameters for generating Root CA and certificates -
  - 2048 bytes long key
  - AES256 key encryption using a password
  - SHA256 as digest algorithm for signing requests
  - `Subject Common Name` is mandatory for the client certificate

Server is configured with TLS 1.3 as the minimum required version. The following ciphers are
configured on the server -

  - `TLS_AES_128_GCM_SHA256`
  - `TLS_AES_256_GCM_SHA384`
  - `TLS_CHACHA20_POLY1305_SHA256`

### Authorization

As mTLS is used for authentication, the server can identify the client using the `Subject Common
Name` field in the client certificate. When a job is started by a client, its `CN` from the client
certificate is stored as the ownership identifier for the job by the server. The server will verify
the `CN` from the client certificate for subsequence operations on the job to make sure that same
client is accessing the job.
