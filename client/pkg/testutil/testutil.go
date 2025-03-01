// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package testutil provides test utility functions.
package testutil

import (
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"
)

// WaitSchedule briefly sleeps in order to invoke the go scheduler.
// TODO: improve this when we are able to know the schedule or status of target go-routine.
func WaitSchedule() {
	time.Sleep(10 * time.Millisecond)
}

func MustNewURLs(t *testing.T, urls []string) []url.URL {
	t.Helper()
	if urls == nil {
		return nil
	}
	var us []url.URL
	for _, url := range urls {
		u := MustNewURL(t, url)
		us = append(us, *u)
	}
	return us
}

func MustNewURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %v error: %v", s, err)
	}
	return u
}

// FatalStack helps to fatal the test and print out the stacks of all running goroutines.
func FatalStack(t *testing.T, s string) {
	t.Helper()
	stackTrace := make([]byte, 1024*1024)
	n := runtime.Stack(stackTrace, true)
	t.Errorf("---> Test failed: %s", s)
	t.Error(string(stackTrace[:n]))
	t.Fatal(s)
}

// ConditionFunc returns true when a condition is met.
type ConditionFunc func() (bool, error)

// Poll calls a condition function repeatedly on a polling interval until it returns true, returns an error
// or the timeout is reached. If the condition function returns true or an error before the timeout, Poll
// immediately returns with the true value or the error. If the timeout is exceeded, Poll returns false.
func Poll(interval time.Duration, timeout time.Duration, condition ConditionFunc) (bool, error) {
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			return false, nil
		case <-ticker.C:
			success, err := condition()
			if err != nil {
				return false, err
			}
			if success {
				return true, nil
			}
		}
	}
}

func SkipTestIfShortMode(t TB, reason string) {
	if t != nil {
		t.Helper()
		if testing.Short() {
			t.Skip(reason)
		}
	}
}

// ExitInShortMode closes the current process (with 0) if the short test mode detected.
//
// To be used in Test-main, where test context (testing.TB) is not available.
//
// Requires custom env-variable (GOLANG_TEST_SHORT) apart of `go test --short flag`.
func ExitInShortMode(reason string) {
	if os.Getenv("GOLANG_TEST_SHORT") == "true" {
		os.Exit(0)
	}
}
