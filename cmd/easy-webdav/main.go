package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/122440367/easy-webdav/internal/admin"
	"github.com/122440367/easy-webdav/internal/config"
	"github.com/122440367/easy-webdav/internal/privdrop"
	"github.com/122440367/easy-webdav/internal/quota"
	"github.com/122440367/easy-webdav/internal/server"
	"github.com/122440367/easy-webdav/internal/store"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

func main() {
	args := os.Args[1:]
	printConfig := slices.Contains(args, "--print-config")
	printVersion := slices.Contains(args, "--version")
	if len(args) > 0 {
		switch args[0] {
		case "serve":
			args = args[1:]
		case "admin":
			// Offline account management: create users, recover a lost
			// administrator password, list accounts.
			os.Exit(admin.Run(args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr))
		case "version":
			printVersion = true
		case "config":
			if len(args) < 2 || args[1] != "print" {
				fmt.Fprintln(os.Stderr, "usage: easy-webdav config print [flags]")
				os.Exit(2)
			}
			printConfig = true
			args = args[2:]
		case "healthcheck":
			// What the container HEALTHCHECK calls: no shell or extra tools
			// are needed inside the image.
			os.Exit(healthcheck(args[1:]))
		}
	}
	if printVersion {
		fmt.Printf("easy-webdav %s (%s, %s)\n", version, commit, built)
		return
	}
	effective, err := config.Load(args, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if printConfig {
		output, err := config.PrintJSON(effective)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println(string(output))
		return
	}
	// Containers may start as root only to hand the data directory to the
	// account named by EW_PUID/EW_PGID; every other deployment skips this.
	if err := privdrop.MaybeDrop(effective.Config.DataDir); err != nil {
		fmt.Fprintln(os.Stderr, "privilege drop:", err)
		os.Exit(1)
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

// healthcheck probes the local /healthz endpoint and reports the result
// through the process exit code.
func healthcheck(args []string) int {
	effective, err := config.Load(args, os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	host, port, err := net.SplitHostPort(effective.Config.Listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port) + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: %s returned %s\n", url, response.Status)
		return 1
	}
	return 0
}
