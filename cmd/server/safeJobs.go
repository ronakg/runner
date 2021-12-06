package main

import (
	"sync"

	"github.com/ronakg/runner/pkg/lib"
)

type safeJobs struct {
	table map[string]lib.Job
	sync.RWMutex
}

func (sj *safeJobs) Set(key string, job lib.Job) {
	sj.Lock()
	defer sj.Unlock()

	sj.table[key] = job
}

func (sj *safeJobs) Get(key string) (job lib.Job, ok bool) {
	sj.RLock()
	defer sj.RUnlock()

	job, ok = sj.table[key]
	return
}
