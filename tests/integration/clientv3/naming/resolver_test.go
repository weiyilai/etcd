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

package naming_test

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	testpb "google.golang.org/grpc/interop/grpc_testing"

	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"go.etcd.io/etcd/pkg/v3/grpctesting"
	integration2 "go.etcd.io/etcd/tests/v3/framework/integration"
)

func testEtcdGRPCResolver(t *testing.T, lbPolicy string) {
	// Setup two new dummy stub servers
	payloadBody := []byte{'1'}
	s1 := grpctesting.NewDummyStubServer(payloadBody)
	if err := s1.Start(nil); err != nil {
		t.Fatal("failed to start dummy grpc server (s1)", err)
	}
	defer s1.Stop()

	s2 := grpctesting.NewDummyStubServer(payloadBody)
	if err := s2.Start(nil); err != nil {
		t.Fatal("failed to start dummy grpc server (s2)", err)
	}
	defer s2.Stop()

	// Create new cluster with endpoint manager with two endpoints
	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	em, err := endpoints.NewManager(clus.Client(0), "foo")
	if err != nil {
		t.Fatal("failed to create EndpointManager", err)
	}

	e1 := endpoints.Endpoint{Addr: s1.Addr()}
	e2 := endpoints.Endpoint{Addr: s2.Addr()}

	err = em.AddEndpoint(t.Context(), "foo/e1", e1)
	if err != nil {
		t.Fatal("failed to add foo", err)
	}

	err = em.AddEndpoint(t.Context(), "foo/e2", e2)
	if err != nil {
		t.Fatal("failed to add foo", err)
	}

	b, err := resolver.NewBuilder(clus.Client(1))
	if err != nil {
		t.Fatal("failed to new resolver builder", err)
	}

	// Create connection with provided lb policy
	conn, err := grpc.Dial("etcd:///foo", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithResolvers(b),
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, lbPolicy)))
	if err != nil {
		t.Fatal("failed to connect to foo", err)
	}
	defer conn.Close()

	// Send an initial request that should go to e1
	c := testpb.NewTestServiceClient(conn)
	resp, err := c.UnaryCall(t.Context(), &testpb.SimpleRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatal("failed to invoke rpc to foo (e1)", err)
	}
	if resp.GetPayload() == nil || !bytes.Equal(resp.GetPayload().GetBody(), payloadBody) {
		t.Fatalf("unexpected response from foo (e1): %s", resp.GetPayload().GetBody())
	}

	// Send more requests
	lastResponse := []byte{'1'}
	totalRequests := 3500
	for i := 1; i < totalRequests; i++ {
		resp, err := c.UnaryCall(t.Context(), &testpb.SimpleRequest{}, grpc.WaitForReady(true))
		if err != nil {
			t.Fatal("failed to invoke rpc to foo", err)
		}

		t.Logf("Response: %v", string(resp.GetPayload().GetBody()))

		require.NotNilf(t, resp.GetPayload(), "unexpected response from foo: %s", resp.GetPayload().GetBody())
		lastResponse = resp.GetPayload().GetBody()
	}

	// If the load balancing policy is pick first then return payload should equal number of requests
	t.Logf("Last response: %v", string(lastResponse))
	if lbPolicy == "pick_first" {
		require.Equalf(t, "3500", string(lastResponse), "unexpected total responses from foo: %s", lastResponse)
	}

	// If the load balancing policy is round robin we should see roughly half total requests served by each server
	if lbPolicy == "round_robin" {
		responses, err := strconv.Atoi(string(lastResponse))
		require.NoErrorf(t, err, "couldn't convert to int: %s", lastResponse)

		// Allow 25% tolerance as round robin is not perfect and we don't want the test to flake
		expected := float64(totalRequests) * 0.5
		assert.InEpsilonf(t, expected, float64(responses), 0.25, "unexpected total responses from foo: %s", lastResponse)
	}
}

// TestEtcdGrpcResolverPickFirst mimics scenarios described in grpc_naming.md doc.
func TestEtcdGrpcResolverPickFirst(t *testing.T) {
	integration2.BeforeTest(t)

	// Pick first is the default load balancer policy for grpc-go
	testEtcdGRPCResolver(t, "pick_first")
}

// TestEtcdGrpcResolverRoundRobin mimics scenarios described in grpc_naming.md doc.
func TestEtcdGrpcResolverRoundRobin(t *testing.T) {
	integration2.BeforeTest(t)

	// Round robin is a common alternative for more production oriented scenarios
	testEtcdGRPCResolver(t, "round_robin")
}

func TestEtcdEndpointManager(t *testing.T) {
	integration2.BeforeTest(t)

	s1PayloadBody := []byte{'1'}
	s1 := grpctesting.NewDummyStubServer(s1PayloadBody)
	err := s1.Start(nil)
	require.NoError(t, err)
	defer s1.Stop()

	s2PayloadBody := []byte{'2'}
	s2 := grpctesting.NewDummyStubServer(s2PayloadBody)
	err = s2.Start(nil)
	require.NoError(t, err)
	defer s2.Stop()

	clus := integration2.NewCluster(t, &integration2.ClusterConfig{Size: 3})
	defer clus.Terminate(t)

	// Check if any endpoint with the same prefix "foo" will not break the logic with multiple endpoints
	em, err := endpoints.NewManager(clus.Client(0), "foo")
	require.NoError(t, err)
	emOther, err := endpoints.NewManager(clus.Client(1), "foo_other")
	require.NoError(t, err)

	e1 := endpoints.Endpoint{Addr: s1.Addr()}
	e2 := endpoints.Endpoint{Addr: s2.Addr()}

	em.AddEndpoint(t.Context(), "foo/e1", e1)
	emOther.AddEndpoint(t.Context(), "foo_other/e2", e2)

	epts, err := em.List(t.Context())
	require.NoError(t, err)
	eptsOther, err := emOther.List(t.Context())
	require.NoError(t, err)
	assert.Len(t, epts, 1)
	assert.Len(t, eptsOther, 1)
}
