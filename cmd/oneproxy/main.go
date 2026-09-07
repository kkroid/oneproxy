package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/proxy"
)

const (
	singboxConfigFile = "singbox_generated.json"
	usageText         = "Usage: oneproxy --config <path>\n"
)

type cliOptions struct {
	configPath string
	help       bool
}

type runtimePaths struct {
	configPath      string
	executableDir   string
	singboxPath     string
	stateDir        string
	generatedConfig string
	logPath         string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	if opts.help {
		fmt.Fprint(stdout, usageText)
		fmt.Fprintln(stdout, "Run OneProxy in the foreground using the specified configuration file.")
		return 0
	}

	cwd, err := os.Getwd()
	if err != nil {
		return runtimeError(stderr, "resolve working directory", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return runtimeError(stderr, "resolve home directory", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return runtimeError(stderr, "resolve OneProxy executable", err)
	}
	paths, err := resolveRuntimePaths(opts.configPath, cwd, home, executable)
	if err != nil {
		return runtimeError(stderr, "resolve runtime paths", err)
	}

	if err := validateInputConfig(paths.configPath); err != nil {
		return runtimeError(stderr, "validate config", err)
	}
	cfg, err := config.Load(paths.configPath)
	if err != nil {
		return runtimeError(stderr, "load config", err)
	}
	if err := ensurePrivateDir(paths.stateDir); err != nil {
		return runtimeError(stderr, "prepare state directory", err)
	}
	if err := ensurePrivateDir(filepath.Dir(paths.logPath)); err != nil {
		return runtimeError(stderr, "prepare log directory", err)
	}
	if err := config.NewSingBoxGenerator(cfg, paths.executableDir).SaveToFile(paths.generatedConfig); err != nil {
		return runtimeError(stderr, "generate sing-box config", err)
	}

	manager := proxy.NewForegroundManagerWithLog(paths.singboxPath, paths.generatedConfig, paths.logPath)
	if err := manager.Check(); err != nil {
		return runtimeError(stderr, "check sing-box config", err)
	}

	healthChecker := proxy.NewHealthChecker(cfg, manager)
	if cfg.DNS.FlushOnFailure {
		dnsFlusher := proxy.NewDNSFlusher()
		cooldown := time.Duration(cfg.DNS.FlushIntervalSeconds) * time.Second
		if cooldown <= 0 {
			cooldown = 300 * time.Second
		}
		dnsFlusher.SetCooldown(cooldown)
		healthChecker.SetAllDownCallback(func() {
			if err := dnsFlusher.FlushAll(manager); err != nil {
				fmt.Fprintf(stderr, "DNS flush failed: %v\n", err)
			}
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintln(stdout, "Starting proxy...")
	if err := manager.Start(); err != nil {
		return runtimeError(stderr, "start sing-box", err)
	}

	healthEnabled := cfg.HealthCheck.Enabled && cfg.HealthCheck.IntervalSeconds > 0
	if healthEnabled {
		healthChecker.Start()
		fmt.Fprintf(stdout, "Health check enabled (every %ds)\n", cfg.HealthCheck.IntervalSeconds)
	}
	fmt.Fprintln(stdout, "OneProxy running. Ctrl+C to stop.")

	statusStop := make(chan struct{})
	if healthEnabled {
		go printStatusPeriodically(healthChecker, time.Duration(cfg.HealthCheck.IntervalSeconds)*time.Second, statusStop, stdout)
	}

	select {
	case <-sigCh:
		close(statusStop)
		healthChecker.Stop()
		fmt.Fprintln(stdout, "Shutting down...")
		if err := manager.Stop(); err != nil {
			return runtimeError(stderr, "stop sing-box", err)
		}
		fmt.Fprintln(stdout, "OneProxy stopped")
		return 0
	case err := <-manager.Done():
		close(statusStop)
		healthChecker.Stop()
		if err == nil {
			err = errors.New("sing-box exited unexpectedly")
		}
		return runtimeError(stderr, "sing-box stopped", err)
	}
}

func parseArgs(args []string, stderr io.Writer) (cliOptions, error) {
	var opts cliOptions
	flags := flag.NewFlagSet("oneproxy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	flags.StringVar(&opts.configPath, "config", "", "path to the OneProxy configuration file")
	flags.BoolVar(&opts.help, "help", false, "show help")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			opts.help = true
			return opts, nil
		}
		return cliOptions{}, err
	}
	if opts.help {
		return opts, nil
	}
	if flags.NArg() != 0 {
		return cliOptions{}, fmt.Errorf("unexpected positional argument %q", flags.Arg(0))
	}
	if opts.configPath == "" {
		return cliOptions{}, errors.New("--config is required")
	}
	return opts, nil
}

func resolveRuntimePaths(configPath, cwd, home, executable string) (runtimePaths, error) {
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(cwd, configPath)
	}
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return runtimePaths{}, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return runtimePaths{}, err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return runtimePaths{}, err
	}
	executableDir := filepath.Dir(executable)
	stateDir := filepath.Join(home, ".oneproxy")
	return runtimePaths{
		configPath:      configPath,
		executableDir:   executableDir,
		singboxPath:     filepath.Join(executableDir, "bin", singBoxExecutableName()),
		stateDir:        stateDir,
		generatedConfig: filepath.Join(stateDir, singboxConfigFile),
		logPath:         filepath.Join(stateDir, "logs", "singbox.log"),
	}, nil
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func printStatusPeriodically(hc *proxy.HealthChecker, interval time.Duration, stop <-chan struct{}, stdout io.Writer) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			printStatus(hc, stdout)
		case <-stop:
			return
		}
	}
}

func printStatus(hc *proxy.HealthChecker, stdout io.Writer) {
	fmt.Fprintf(stdout, "\n[%s] --- Health ---\n", time.Now().Format("15:04:05"))
	for name, result := range hc.GetAllResults() {
		if result.IsHealthy {
			fmt.Fprintf(stdout, "  [OK] %-16s :%d  %dms\n", name, result.LocalPort, result.Latency.Milliseconds())
		} else {
			fmt.Fprintf(stdout, "  [!!] %-16s :%d  %s\n", name, result.LocalPort, result.LastError)
		}
	}
}

func runtimeError(stderr io.Writer, action string, err error) int {
	fmt.Fprintf(stderr, "oneproxy: %s: %v\n", action, err)
	return 1
}
