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

package command

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"go.etcd.io/etcd/pkg/v3/cobrautl"
)

// NewDefragCommand returns the cobra command for "Defrag".
func NewDefragCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defrag",
		Short: "Defragments the storage of the etcd members with given endpoints",
		Run:   defragCommandFunc,
	}
	cmd.PersistentFlags().BoolVar(&epClusterEndpoints, "cluster", false, "use all endpoints from the cluster member list")
	return cmd
}

func defragCommandFunc(cmd *cobra.Command, args []string) {
	failures := 0
	cfg := clientConfigFromCmd(cmd)
	for _, ep := range endpointsFromCluster(cmd) {
		cfg.Endpoints = []string{ep}
		c := mustClient(cfg)
		ctx, cancel := commandCtx(cmd)
		start := time.Now()
		_, err := c.Defragment(ctx, ep)
		d := time.Since(start)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to defragment etcd member[%s]. took %s. (%v)\n", ep, d.String(), err)
			failures++
		} else {
			fmt.Printf("Finished defragmenting etcd member[%s]. took %s\n", ep, d.String())
		}
		c.Close()
	}

	if failures != 0 {
		os.Exit(cobrautl.ExitError)
	}
}
