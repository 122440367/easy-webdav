package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/lecritus/easy-webdav/internal/config"
	"github.com/lecritus/easy-webdav/internal/quota"
	"github.com/lecritus/easy-webdav/internal/server"
	"github.com/lecritus/easy-webdav/internal/store"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

func main() {
	if slices.Contains(os.Args[1:], "--version") {
		fmt.Printf("easy-webdav %s (%s, %s)\n", version, commit, built)
		return
	}
	effective, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if slices.Contains(os.Args[1:], "--print-config") {
		output, err := config.PrintJSON(effective)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println(string(output))
		return
	}
	if err := config.EnsureDirs(effective.Config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	db, err := store.Open(filepath.Join(effective.Config.DataDir, "easy-webdav.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	srv := server.New(effective.Config, db)
	if err := srv.Auth.Bootstrap(context.Background(), effective.Config.AdminUser, effective.Config.AdminPassword); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if err := quota.Recalculate(db, effective.Config.StorageDir); err != nil { fmt.Fprintln(os.Stderr, "usage recalculation:", err) }
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { ticker:=time.NewTicker(24*time.Hour); defer ticker.Stop(); for { select { case <-ticker.C: if err:=quota.Recalculate(db,effective.Config.StorageDir);err!=nil{fmt.Fprintln(os.Stderr,"usage recalculation:",err)}; case <-ctx.Done(): return } } }()
	go func() {
		serve := srv.ListenAndServe
		if effective.Config.TLSCert != "" { serve = srv.ListenAndServeTLS }
		if err := serve(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
			stop()
		}
	}()
	fmt.Printf("easy-webdav %s listening on %s\n", version, effective.Config.Listen)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
