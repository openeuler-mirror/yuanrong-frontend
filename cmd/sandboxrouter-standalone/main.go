// Standalone sandboxrouter for PoC: runs the InstanceInfo-watch reverse proxy
// as a separate process (no faasfrontend redeploy), watching the cluster etcd.
//
//	ROUTER_ETCD=172.21.0.2:32379 ROUTER_PORT=8080 \
//	ROUTER_JWT_AUTH=1 ROUTER_RRT_PORT=50090 [ROUTER_VALIDATE_IAM=1] \
//	sandboxrouter-standalone
package main

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"frontend/pkg/common/faas_common/alarm"
	"frontend/pkg/common/faas_common/etcd3"
	"frontend/pkg/sandboxrouter"
	"frontend/pkg/sandboxrouter/config"
)

func main() {
	etcdAddr := os.Getenv("ROUTER_ETCD")
	if etcdAddr == "" {
		etcdAddr = "172.21.0.2:32379"
	}
	port := 8080
	if p, err := strconv.Atoi(os.Getenv("ROUTER_PORT")); err == nil && p > 0 {
		port = p
	}

	stopCh := make(chan struct{})
	ec := etcd3.EtcdConfig{Servers: []string{etcdAddr}, AuthType: "Noauth"}
	if err := etcd3.InitRouterEtcdClient(ec, alarm.Config{}, stopCh); err != nil {
		panic(err)
	}

	cfg := &config.SandboxRouterConfig{Enabled: true, ListenIP: "0.0.0.0", ListenPort: port}
	if envTrue("ROUTER_JWT_AUTH") {
		cfg.EnableJWTAuth = true
		cfg.ValidateIAM = envTrue("ROUTER_VALIDATE_IAM")
		cfg.RRTPort = 50090
		if p, err := strconv.Atoi(os.Getenv("ROUTER_RRT_PORT")); err == nil && p > 0 {
			cfg.RRTPort = p
		}
	}
	r, err := sandboxrouter.New(cfg)
	if err != nil {
		panic(err)
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		close(stopCh)
	}()

	if err := r.Start(stopCh); err != nil {
		panic(err)
	}
}

func envTrue(name string) bool {
	switch os.Getenv(name) {
	case "1", "true", "yes", "TRUE", "True":
		return true
	default:
		return false
	}
}
