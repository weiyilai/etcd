// Copyright 2018 The etcd Authors
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

package snapshot_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/client/pkg/v3/testutil"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/snapshot"
	"go.etcd.io/etcd/server/v3/embed"
	integration2 "go.etcd.io/etcd/tests/v3/framework/integration"
)

// TestSaveSnapshotFilePermissions ensures that the snapshot is saved with
// the correct file permissions.
func TestSaveSnapshotFilePermissions(t *testing.T) {
	expectedFileMode := os.FileMode(fileutil.PrivateFileMode)
	kvs := []kv{{"foo1", "bar1"}, {"foo2", "bar2"}, {"foo3", "bar3"}}
	_, dbPath := createSnapshotFile(t, newEmbedConfig(t), kvs)
	defer os.RemoveAll(dbPath)

	dbInfo, err := os.Stat(dbPath)
	require.NoErrorf(t, err, "failed to get test snapshot file status: %v", err)
	actualFileMode := dbInfo.Mode()

	require.Equalf(t, expectedFileMode, actualFileMode, "expected test snapshot file mode %s, got %s:", expectedFileMode, actualFileMode)
}

// TestSaveSnapshotVersion ensures that the snapshot returns proper storage version.
func TestSaveSnapshotVersion(t *testing.T) {
	// Put some keys to ensure that wal snapshot is triggered
	var kvs []kv
	for i := 0; i < 10; i++ {
		kvs = append(kvs, kv{fmt.Sprintf("%d", i), "test"})
	}
	cfg := newEmbedConfig(t)
	// Force raft snapshot to ensure that storage version is set
	cfg.SnapshotCount = 1
	ver, dbPath := createSnapshotFile(t, cfg, kvs)
	defer os.RemoveAll(dbPath)

	require.Equalf(t, "3.7.0", ver, "expected snapshot version %s, got %s:", "3.7.0", ver)
}

type kv struct {
	k, v string
}

func newEmbedConfig(t *testing.T) *embed.Config {
	clusterN := 1
	urls := newEmbedURLs(clusterN * 2)
	cURLs, pURLs := urls[:clusterN], urls[clusterN:]
	cfg := integration2.NewEmbedConfig(t, "default")
	cfg.ClusterState = "new"
	cfg.ListenClientUrls, cfg.AdvertiseClientUrls = cURLs, cURLs
	cfg.ListenPeerUrls, cfg.AdvertisePeerUrls = pURLs, pURLs
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, pURLs[0].String())
	return cfg
}

// creates a snapshot file and returns the file path.
func createSnapshotFile(t *testing.T, cfg *embed.Config, kvs []kv) (version string, dbPath string) {
	testutil.SkipTestIfShortMode(t,
		"Snapshot creation tests are depending on embedded etcd server so are integration-level tests.")

	srv, err := embed.StartEtcd(cfg)
	require.NoError(t, err)
	defer func() {
		srv.Close()
	}()
	select {
	case <-srv.Server.ReadyNotify():
	case <-time.After(3 * time.Second):
		t.Fatalf("failed to start embed.Etcd for creating snapshots")
	}

	ccfg := clientv3.Config{Endpoints: []string{cfg.AdvertiseClientUrls[0].String()}}
	cli, err := integration2.NewClient(t, ccfg)
	require.NoError(t, err)
	defer cli.Close()
	for i := range kvs {
		ctx, cancel := context.WithTimeout(t.Context(), testutil.RequestTimeout)
		_, err = cli.Put(ctx, kvs[i].k, kvs[i].v)
		cancel()
		require.NoError(t, err)
	}

	dbPath = filepath.Join(t.TempDir(), fmt.Sprintf("snapshot%d.db", time.Now().Nanosecond()))
	version, err = snapshot.SaveWithVersion(t.Context(), zaptest.NewLogger(t), ccfg, dbPath)
	require.NoError(t, err)
	return version, dbPath
}

func newEmbedURLs(n int) (urls []url.URL) {
	urls = make([]url.URL, n)
	for i := 0; i < n; i++ {
		u, _ := url.Parse(fmt.Sprintf("unix://localhost:%d", rand.Intn(45000)))
		urls[i] = *u
	}
	return urls
}
