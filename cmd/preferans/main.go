package main

import "preferans/locales"

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"preferans/config"
	"preferans/internal/app"
	"preferans/internal/update"
	"preferans/netcheck"
	"preferans/web"
)

var appMode = "game"
var appLanguage = "uk"

func main() {
	buildInfo := flag.Bool("build-info", false, "Print build capabilities and exit")
	headless := flag.Bool("headless", false, "Run local UI HTTP endpoint for development")
	dev := flag.Bool("dev", false, "Open local UI in Chrome for development")
	data := flag.String("data", "", "Data directory")
	check := flag.Bool("netcheck", appMode == "netcheck", "Network diagnostic UI")
	var logPath string
	flag.StringVar(&logPath, "c", "", "Path to append-only diagnostic log")
	flag.StringVar(&logPath, "config", "", "Path to append-only diagnostic log")
	updatePort := flag.Int("update-port", 8787, "LAN port for Android update files")
	flag.Parse()
	if *buildInfo {
		fmt.Printf("{\"aiEnabled\":%t}\n", config.AIEnabled)
		return
	}
	if *data == "" {
		d, e := os.UserConfigDir()
		if e != nil {
			log.Fatal(e)
		}
		name := "Preferans"
		if *check {
			name = "PreferansNetCheck"
		}
		*data = filepath.Join(d, name)
	}
	a := app.New(*data)
	appLanguage = a.Status().Appearance.Language
	if appLanguage == "" {
		appLanguage = "uk"
	}
	if logPath != "" {
		if e := a.SetLogFile(logPath); e != nil {
			log.Fatal(e)
		}
	}
	assets := web.Files()
	if *check {
		a.EnableDiagnostics()
		assets = netcheck.Files()
		appMode = "netcheck"
	}
	url, stop, e := app.Serve(a, assets)
	if e != nil {
		log.Fatal(e)
	}
	defer stop()
	var updateServer *update.Server
	if *updatePort >= 0 {
		updateRoot := filepath.Dir(os.Args[0])
		if executable, e := os.Executable(); e == nil {
			updateRoot = filepath.Dir(executable)
		} else {
			log.Printf(locales.Text("go.cmd.preferans.main.text001"), e, updateRoot)
		}
		if updateServer, e = update.Start(updateRoot, *updatePort); e != nil {
			log.Printf(locales.Text("go.cmd.preferans.main.text002"), e)
		} else {
			updateServer.SetAITransfer(a.AITransferHandler())
			defer updateServer.Close()
		}
	}
	if *dev {
		fmt.Println(url)
		if e = openDevBrowser(url); e != nil {
			log.Fatal(e)
		}
		waitForInterrupt()
		return
	}
	if !*headless {
		if e = window(url); e != nil {
			log.Fatal(e)
		}
		return
	}
	fmt.Println(url)
	waitForInterrupt()
}

func waitForInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
