/*
 * JuiceFS, Copyright 2018 Juicedata, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jacksonbean/juicefs-sync-advanced/pkg/metric"
	"github.com/jacksonbean/juicefs-sync-advanced/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

// InstanceInfo represents a running juicefs-sync instance for UI discovery.
type InstanceInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Metrics   string `json:"metrics"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Name      string `json:"name"`
	StartTime string `json:"start_time"`
}

var (
	instanceRegistryPath = filepath.Join(os.Getenv("HOME"), ".juicefs_sync_instances.json")
	registryMutex        sync.Mutex
)

// exposeMetrics starts the Prometheus metrics HTTP server and registers
// this instance in the shared registry file for UI discovery.
func exposeMetrics(c *cli.Context, registerer prometheus.Registerer, registry *prometheus.Registry) string {
	ip, port, err := net.SplitHostPort(c.String("metrics"))
	if err != nil {
		logger.Fatalf("metrics format error: %v", err)
	}
	go metric.UpdateMetrics(registerer)
	http.Handle("/metrics", promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	))
	registerer.MustRegister(collectors.NewBuildInfoCollector())

	if !c.IsSet("metrics") {
		if c.IsSet("consul") {
			ip, err = utils.GetLocalIp(c.String("consul"))
			if err != nil {
				logger.Errorf("Get local ip failed: %v", err)
				return ""
			}
		}
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(ip, port))
	if err != nil {
		if c.IsSet("metrics") {
			logger.Errorf("listen on %s:%s failed: %v", ip, port, err)
			return ""
		}
		ln, err = net.Listen("tcp", net.JoinHostPort(ip, "0"))
		if err != nil {
			logger.Errorf("Listen failed: %v", err)
			return ""
		}
	}

	go func() {
		if err := http.Serve(ln, nil); err != nil {
			logger.Errorf("Serve for metrics: %s", err)
		}
	}()

	metricsAddr := ln.Addr().String()
	logger.Infof("Prometheus metrics listening on %s", metricsAddr)

	// Register in shared registry for multi-instance UI discovery
	hostPort := ln.Addr().String()
	actualPort := 9567
	if parts := strings.Split(hostPort, ":"); len(parts) == 2 {
		if p, err := strconv.Atoi(parts[1]); err == nil {
			actualPort = p
		}
	}

	// Use cli context Args() and String() (much more reliable than parsing os.Args)
	src := utils.RemovePassword(c.Args().Get(0))
	dst := utils.RemovePassword(c.Args().Get(1))
	name := c.String("instance-name")

	registerInstance(InstanceInfo{
		PID:       os.Getpid(),
		Port:      actualPort,
		Metrics:   fmt.Sprintf("http://localhost:%d/metrics", actualPort),
		Src:       src,
		Dst:       dst,
		Name:      name,
		StartTime: time.Now().Format(time.RFC3339),
	})

	// Clean up registry on signal (SIGINT/SIGTERM) or on process death
	// (stale entries are filtered by PID liveness check on the UI side)
	catchCleanup(func() {
		unregisterInstance(os.Getpid())
	})

	return metricsAddr
}

func registerInstance(info InstanceInfo) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	withRegistryFileLock(func() {
		instances := loadInstances()

		found := false
		for i, inst := range instances {
			if inst.PID == info.PID {
				instances[i] = info
				found = true
				break
			}
		}
		if !found {
			instances = append(instances, info)
		}

		saveInstances(instances)
	})
	logger.Debugf("Registered instance PID=%d metrics=%s", info.PID, info.Metrics)
}

func unregisterInstance(pid int) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	withRegistryFileLock(func() {
		instances := loadInstances()
		filtered := make([]InstanceInfo, 0, len(instances))
		for _, inst := range instances {
			if inst.PID != pid {
				filtered = append(filtered, inst)
			}
		}
		saveInstances(filtered)
	})
	logger.Debugf("Unregistered instance PID=%d", pid)
}

// withRegistryFileLock acquires an exclusive flock on the registry file,
// calls fn, then releases the lock. This prevents cross-process races
// when multiple juicefs-sync instances register/unregister simultaneously.
func withRegistryFileLock(fn func()) {
	f, err := os.OpenFile(instanceRegistryPath, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		logger.Warnf("Failed to open registry lock file: %v", err)
		fn()
		return
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		logger.Warnf("Failed to acquire registry lock: %v", err)
		fn()
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	fn()
}

func loadInstances() []InstanceInfo {
	data, err := os.ReadFile(instanceRegistryPath)
	if err != nil {
		return []InstanceInfo{}
	}
	var instances []InstanceInfo
	if err := json.Unmarshal(data, &instances); err != nil {
		return []InstanceInfo{}
	}
	return instances
}

func saveInstances(instances []InstanceInfo) {
	data, err := json.MarshalIndent(instances, "", "  ")
	if err != nil {
		logger.Errorf("Failed to marshal instance registry: %v", err)
		return
	}
	if err := os.WriteFile(instanceRegistryPath, data, 0644); err != nil {
		logger.Errorf("Failed to write instance registry: %v", err)
	}
}

func catchCleanup(cleanup func()) {
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigCh
		cleanup()
	}()
}
