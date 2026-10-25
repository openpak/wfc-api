package main

import (
	"context"
	"fmt"
	"net/http"
	"openpak/wfc-api/api"
	"openpak/wfc-api/corebridge"
	"openpak/wfc-common/common"
	"openpak/wfc-common/logging"
	"openpak/wfc-common/service"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := common.LoadTokenKey(); e != nil {
		panic(e)
	}
	cfg := common.GetConfig()
	logging.SetLevel(*cfg.LogLevel)
	cfg.RegisterWebhooks()
	common.ReadGameList()
	os.MkdirAll("state", 0700)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	api.StartServer(false)
	defer api.Shutdown()
	mux := http.NewServeMux()
	api.RegisterHandlers(mux)
	corebridge.Start(cfg.CoreAddress, cfg.CoreKey, fmt.Sprintf("postgres://%s:%s@%s/%s", cfg.Username, cfg.Password, cfg.DatabaseAddress, cfg.DatabaseName))
	h := http.NewServeMux()
	h.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	// Operator endpoints retain their existing API-secret role checks so the
	// deployed website can call its direct owner without a compatibility proxy.
	h.Handle("/", mux)
	addr := os.Getenv("WFC_HTTP_ADDR")
	if addr == "" {
		addr = ":20209"
	}
	if e := service.Run(ctx, service.Server(addr, h)); e != nil {
		panic(e)
	}
}
