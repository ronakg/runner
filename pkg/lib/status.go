package lib

import "sync"

// JobStatus represents the status of a job
type JobStatus int

func (s JobStatus) String() string {
	switch s {
	case StatusRunning:
		return "RUNNING"
	case StatusCompleted:
		return "COMPLETED"
	case StatusTimedOut:
		return "TIMEDOUT"
	case StatusStopped:
		return "STOPPED"
	}
	return "UNKNOWN"
}

const (
	// StatusCreated denotes a newly created job that's not yet started
	StatusCreated JobStatus = iota - 1
	// StatusRunning denotes a job that's in running state
	StatusRunning
	// StatusCompleted denotes a job that ran to its completion
	StatusCompleted
	// StatusStopped denotes a job that was stopped by the caller
	StatusStopped
	// StatusTimedOut denotes a job that was killed due to timeout expiration
	StatusTimedOut
)

// safeJobStatus provides a safer way to use JobStatus protecting it with a lock
type safeJobStatus struct {
	value JobStatus
	sync.RWMutex
}

// Set sets the JobStatus to new value
func (s *safeJobStatus) Set(new JobStatus) {
	s.Lock()
	defer s.Unlock()

	s.value = new
}

// Get returns the current JobStatus
func (s *safeJobStatus) Get() JobStatus {
	s.RLock()
	defer s.RUnlock()

	return s.value
}

// UpdateIf updates the JobStatus to new value only if the existing value is set to old
func (s *safeJobStatus) UpdateIf(old JobStatus, new JobStatus) {
	s.Lock()
	defer s.Unlock()

	if s.value == old {
		s.value = new
	}
}
