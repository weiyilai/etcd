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

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.etcd.io/etcd/pkg/v3/expect"
	"go.etcd.io/etcd/tests/v3/framework/e2e"
)

var defaultGatewayEndpoint = "127.0.0.1:23790"

func TestGateway(t *testing.T) {
	ec, err := e2e.NewEtcdProcessCluster(t.Context(), t)
	require.NoError(t, err)
	defer ec.Stop()

	eps := strings.Join(ec.EndpointsGRPC(), ",")

	p := startGateway(t, eps)
	defer func() {
		p.Stop()
		p.Close()
	}()

	err = e2e.SpawnWithExpect([]string{e2e.BinPath.Etcdctl, "--endpoints=" + defaultGatewayEndpoint, "put", "foo", "bar"}, expect.ExpectedResponse{Value: "OK\r\n"})
	if err != nil {
		t.Errorf("failed to finish put request through gateway: %v", err)
	}
}

func startGateway(t *testing.T, endpoints string) *expect.ExpectProcess {
	p, err := expect.NewExpect(e2e.BinPath.Etcd, "gateway", "--endpoints="+endpoints, "start")
	require.NoError(t, err)
	_, err = p.Expect("ready to proxy client requests")
	require.NoError(t, err)
	return p
}
