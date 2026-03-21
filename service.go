package main

import (
	"context"
	"log"

	"github.com/kardianos/service"
)

// headlessProgram 无 WebView / 无托盘，仅跑 initBackend 与可选 MITM（供系统服务 / systemd 等）。
type headlessProgram struct {
	app    *App
	cancel context.CancelFunc
}

func (p *headlessProgram) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.app = NewApp()
	p.app.ctx = ctx
	go func() {
		if err := p.app.initBackend(); err != nil {
			log.Printf("[WindsurfTools] service init: %v", err)
			cancel()
			return
		}
		if p.app.store.GetSettings().MitmProxyEnabled {
			if err := p.app.StartMitmProxy(); err != nil {
				log.Printf("[WindsurfTools] MITM start: %v", err)
			}
		}
		<-ctx.Done()
	}()
	return nil
}

func (p *headlessProgram) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.app != nil {
		p.app.shutdown(context.Background())
	}
	return nil
}

func runHeadlessDaemon() error {
	prg := &headlessProgram{}
	cfg := &service.Config{
		Name:        "WindsurfTools",
		DisplayName: "Windsurf Tools",
		Description: "Windsurf 号池、自动刷新与 MITM（无界面；配置见用户配置目录下 WindsurfTools/settings.json）",
	}
	s, err := service.New(prg, cfg)
	if err != nil {
		return err
	}
	return s.Run()
}

func runServiceControl(action string) error {
	prg := &headlessProgram{}
	cfg := &service.Config{
		Name:        "WindsurfTools",
		DisplayName: "Windsurf Tools",
		Description: "Windsurf 号池、自动刷新与 MITM（无界面；配置见用户配置目录下 WindsurfTools/settings.json）",
	}
	s, err := service.New(prg, cfg)
	if err != nil {
		return err
	}
	return service.Control(s, action)
}
