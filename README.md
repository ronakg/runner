# Runner

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

* [ ] One or more concurrent clients should be able to perform the following operations on a job concurrently
    - [ ] Start
    - [ ] Stop
    - [ ] Query status
    - [ ] Stream output
