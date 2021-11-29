# Runner

Runner is a tool to easily run and manage jobs on a Linux machine. This document describes the
functional and design aspects of Runner.

## Requirements

### Library

* [ ] A method to perform the following operations on a job
    - [ ] Start
    - [ ] Stop
    - [ ] Query status
    - [ ] Stream output
* [ ] Output of a job should always be from the beginning of the job
* [ ] Multiple clients should be able to concurrently retrieve output of a running job
* [ ] Resource control per job using cgroups
    - [ ] CPU
    - [ ] Memory
    - [ ] Disk IO
* [ ] Resource isolation for using PID, mount and networking namespaces

### Server

* [ ] gRPC API to perform the following operations on a job
    - [ ] Start
    - [ ] Stop
    - [ ] Query status
    - [ ] Stream output
* [ ] mTLS must be used for authentication between the client(s) and the server
    - [ ] A strong set of cipher suits must be used
* [ ] A simple authorization scheme is enough for the scope of this project

### Client

* [ ] One or more clients should be able to perform the following operations on a job concurrently
    - [ ] Start
    - [ ] Stop
    - [ ] Query status
    - [ ] Stream output

### Misc

* [ ] Build script to perform the following operations
    - [ ] Build client and server binaries for Linux platform
    - [ ] Run tests on the entire codebase
    - [ ] Generate code coverage report


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
  - `path`: The path to the command/executable that needs to be executed as the job.
  - `args`: Arguments to be passed to the command/executable.
  creation. Client must provide the job id for subsequent operations on the job.
  - `timeout`: Timeout duration in seconds to wait for the job to complete. If the job doesn't
  complete within the specified timeout, the job will be stopped by the library. A timeout of 0
  seconds is interpreted as no timeout.
  - `profile`: The resource profile to be applied to the job. Only 1 profile is supported - `default`.

The following attributes are maintained by the library for each job:
  - `id`: The unique identifier of the job. It is generated automatically by the library upon job
  - `status`: The status of the job.
  - `exitCode`: The exit code of the job.
  - `outFile`: The path to the file where output of the job is written to. File path is of the
  format `/tmp/runner/<job-id>`.

##### Public interfaces

The following public interfaces are exposed by the library package.

```go
// JobConfig represents the input configuration for a job
type JobConfig struct {
  Path    string
  Args    []string
  Timeout time.Duration
  Profile string
}

type JobStatus int
const (
  // StatusUnknown is returned when the status of the job cannot be determined
  StatusUnknown JobStatus = iota
  // StatusCreated is returned when job is created but not yet started
  StatusCreated
  // StatusRunning is returned when the job is still running
  StatusRunning
  // StatusCompleted is returned when the job runs to its completion itself
  StatusCompleted
  // StatusStopped is returned when the job is stopped by the user
  StatusStopped
  // StatusKilled is returned when the timeout expires and the job is killed
  StatusKilled
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

  // Start starts a newly created job
  Start() error

  // Stop stops a running job
  Stop()

  // Status returns current JobStatus and an exit code in case the job has terminated
  Status() (JobStatus, int)

  // Output returns an out channel from which the output of a job can be consumed. The cancel
  // function can be used to stop streaming output from the job. Once cancel function is invoked,
  // the out channel is closed.
  Output() (out <-chan *Output, cancel func(), err error)
}

// NewJob creates and returns a new Job
func NewJob(c Config) (Job, error) {}

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

#### Job Execution

- Start: An [exec Cmd with Context](https://pkg.go.dev/os/exec#CommandContext) is used to start a
  job. The type of context used for the `Cmd` depends on the timeout supplied by the user. When the
  timeout is set to a valid duration, a [Context with Timeout](https://pkg.go.dev/context#WithTimeout)
  is used. When the timeout is set to 0 (which is same as no timeout), [Context with
  Cancel](https://pkg.go.dev/context#WithCancel) is used.

  Once a job is started successfully, the PID of the job is added to the cgroup created for the job's profile.

- Stop: The  `cancel` function of a job's context is used to stop the job. Once a job is terminated
  either on its own or through a `stop` action - the PID of the job is removed from the cgroup
  created for the job's profile.

#### Job Resource Limitation 

Each job created using the library is resource limited using cgroups. The library combines all the
resource control limitations of a job into a profile that defines what limits should be applied for
cpu, memory and disk IO for the job. At the moment, only 1 profile called `default` is supported by
the library.

A new cgroup is created for each resource profile. Naming convention for the resource profile is
`runner-<profile_name>`.

Example cgroup `procs` files for `runner-default` profile -
  - `/sys/fs/cgroup/cpu/runner-default/cgroup.procs`
  - `/sys/fs/cgroup/memory/runner-default/cgroup.procs`
  - `/sys/fs/cgroup/blkio/runner-default/cgroup.procs`

TODO: Decide the actual limits for cpu, memory and blkio.

#### Job Resource Isolation

Library isolates each job by creating a separate namespace for PID, mount and network. This is
achieved by setting `CLONE_NEWPID`, `CLONE_NEWNS` and `CLONE_NEWNET` clone flags during the job
creation.

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

  - `path`: The path to the command that should be executed
  - `args`: The arguments to be passed to the command
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

Server is configured with TLS 1.2 as the minimum required version. The following ciphers are
configured on the server -

  - `TLS_DHE_RSA_WITH_AES_256_CBC_SHA`
  - `TLS_DHE_DSS_WITH_AES_256_CBC_SHA`
  - `TLS_RSA_WITH_AES_256_CBC_SHA`
  - `TLS_DHE_RSA_WITH_3DES_EDE_CBC_SHA`
  - `TLS_DHE_DSS_WITH_3DES_EDE_CBC_SHA`
  - `TLS_RSA_WITH_3DES_EDE_CBC_SHA`
  - `TLS_DHE_RSA_WITH_AES_128_CBC_SHA`
  - `TLS_DHE_DSS_WITH_AES_128_CBC_SHA`
  - `TLS_RSA_WITH_AES_128_CBC_SHA`

### Authorization

As mTLS is used for authentication, the server can identify the client using the `Subject Common
Name` field in the client certificate. When a job is started by a client, its `CN` from the client
certificate is stored as the ownership identifier for the job by the server. The server will verify
the `CN` from the client certificate for subsequence operations on the job to make sure that same
client is accessing the job.
