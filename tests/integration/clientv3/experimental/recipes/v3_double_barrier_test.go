// Copyright 2016 The etcd Authors
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

package recipes_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	recipe "go.etcd.io/etcd/client/v3/experimental/recipes"
	integration2 "go.etcd.io/etcd/tests/v3/framework/integration"
)

func TestDoubleBarrier(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	waiters := 10
	session, err := concurrency.NewSession(clus.RandClient())
	if err != nil {
		t.Error(err)
	}
	defer session.Orphan()

	b := recipe.NewDoubleBarrier(session, "test-barrier", waiters)
	donec := make(chan struct{})
	defer close(donec)
	for i := 0; i < waiters-1; i++ {
		go func() {
			session, err := concurrency.NewSession(clus.RandClient())
			if err != nil {
				t.Error(err)
			}
			defer session.Orphan()

			bb := recipe.NewDoubleBarrier(session, "test-barrier", waiters)
			if err := bb.Enter(); err != nil {
				t.Errorf("could not enter on barrier (%v)", err)
			}
			<-donec
			if err := bb.Leave(); err != nil {
				t.Errorf("could not leave on barrier (%v)", err)
			}
			<-donec
		}()
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case donec <- struct{}{}:
		t.Fatalf("barrier did not enter-wait")
	default:
	}

	require.NoErrorf(t, b.Enter(), "could not enter last barrier")

	timerC := time.After(time.Duration(waiters*100) * time.Millisecond)
	for i := 0; i < waiters-1; i++ {
		select {
		case <-timerC:
			t.Fatalf("barrier enter timed out")
		case donec <- struct{}{}:
		}
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case donec <- struct{}{}:
		t.Fatalf("barrier did not leave-wait")
	default:
	}

	b.Leave()
	timerC = time.After(time.Duration(waiters*100) * time.Millisecond)
	for i := 0; i < waiters-1; i++ {
		select {
		case <-timerC:
			t.Fatalf("barrier leave timed out")
		case donec <- struct{}{}:
		}
	}
}

func TestDoubleBarrierTooManyClients(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	waiters := 10
	session, err := concurrency.NewSession(clus.RandClient())
	if err != nil {
		t.Error(err)
	}
	defer session.Orphan()

	b := recipe.NewDoubleBarrier(session, "test-barrier", waiters)
	donec := make(chan struct{})
	var (
		wgDone    sync.WaitGroup // make sure all clients have finished the tasks
		wgEntered sync.WaitGroup // make sure all clients have entered the double barrier
	)
	wgDone.Add(waiters)
	wgEntered.Add(waiters)
	for i := 0; i < waiters; i++ {
		go func() {
			defer wgDone.Done()

			gsession, gerr := concurrency.NewSession(clus.RandClient())
			if gerr != nil {
				t.Error(gerr)
			}
			defer gsession.Orphan()

			bb := recipe.NewDoubleBarrier(session, "test-barrier", waiters)
			if gerr = bb.Enter(); gerr != nil {
				t.Errorf("could not enter on barrier (%v)", gerr)
			}
			wgEntered.Done()
			<-donec
			if gerr = bb.Leave(); gerr != nil {
				t.Errorf("could not leave on barrier (%v)", gerr)
			}
		}()
	}

	// Wait until all clients have already entered the double barrier, so
	// no any other client can enter the barrier.
	wgEntered.Wait()
	t.Log("Try to enter into double barrier")
	if err = b.Enter(); !errors.Is(err, recipe.ErrTooManyClients) {
		t.Errorf("Unexcepted error, expected: ErrTooManyClients, got: %v", err)
	}

	resp, err := clus.RandClient().Get(t.Context(), "test-barrier/waiters", clientv3.WithPrefix())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// Make sure the extra `b.Enter()` did not create a new ephemeral key
	assert.Len(t, resp.Kvs, waiters)
	close(donec)

	wgDone.Wait()
}

func TestDoubleBarrierFailover(t *testing.T) {
	integration2.BeforeTest(t)

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	waiters := 10
	donec := make(chan struct{})
	defer close(donec)

	s0, err := concurrency.NewSession(clus.Client(0))
	if err != nil {
		t.Error(err)
	}
	defer s0.Orphan()
	s1, err := concurrency.NewSession(clus.Client(0))
	if err != nil {
		t.Error(err)
	}
	defer s1.Orphan()

	// sacrificial barrier holder; lease will be revoked
	go func() {
		b := recipe.NewDoubleBarrier(s0, "test-barrier", waiters)
		if berr := b.Enter(); berr != nil {
			t.Errorf("could not enter on barrier (%v)", berr)
		}
		<-donec
	}()

	for i := 0; i < waiters-1; i++ {
		go func() {
			b := recipe.NewDoubleBarrier(s1, "test-barrier", waiters)
			if berr := b.Enter(); berr != nil {
				t.Errorf("could not enter on barrier (%v)", berr)
			}
			<-donec
			b.Leave()
			<-donec
		}()
	}

	// wait for barrier enter to unblock
	for i := 0; i < waiters; i++ {
		select {
		case donec <- struct{}{}:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for enter, %d", i)
		}
	}

	require.NoError(t, s0.Close())
	// join on rest of waiters
	for i := 0; i < waiters-1; i++ {
		select {
		case donec <- struct{}{}:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for leave, %d", i)
		}
	}
}
