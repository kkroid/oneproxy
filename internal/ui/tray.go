package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/getlantern/systray"
	"github.com/kkroid/oneproxy/internal/config"
	"github.com/kkroid/oneproxy/internal/proxy"
)

// TrayUI manages the system tray interface
type TrayUI struct {
	manager       *proxy.Manager
	config        *config.Config
	healthChecker *proxy.HealthChecker
	dnsFlusher    *proxy.DNSFlusher

	// Menu items
	mStatus          *systray.MenuItem
	mStart           *systray.MenuItem
	mStop            *systray.MenuItem
	mRestart         *systray.MenuItem
	mProxies         *systray.MenuItem
	mHealthCheck     *systray.MenuItem
	mCheckNow        *systray.MenuItem
	mDNSFlush        *systray.MenuItem
	mFlushNow        *systray.MenuItem
	mOpenConfig      *systray.MenuItem
	mViewLogs        *systray.MenuItem
	mQuit            *systray.MenuItem

	// Proxy menu items (for dynamic updates)
	proxyMenuItems map[string]*systray.MenuItem
}

// NewTrayUI creates a new tray UI
func NewTrayUI(manager *proxy.Manager, cfg *config.Config, healthChecker *proxy.HealthChecker, dnsFlusher *proxy.DNSFlusher) *TrayUI {
	return &TrayUI{
		manager:        manager,
		config:         cfg,
		healthChecker:  healthChecker,
		dnsFlusher:     dnsFlusher,
		proxyMenuItems: make(map[string]*systray.MenuItem),
	}
}

// Run starts the tray UI
func (t *TrayUI) Run() {
	systray.Run(t.onReady, t.onExit)
}

// onReady is called when systray is ready
func (t *TrayUI) onReady() {
	// Set icon and title
	systray.SetTitle("OneProxy")
	systray.SetTooltip("OneProxy - Proxy Manager")

	// Set icon (red for stopped)
	t.setIcon(false)

	// Build menu
	t.buildMenu()

	// Start event loop
	go t.eventLoop()

	// Start UI update loop (update health status every 5 seconds)
	go t.uiUpdateLoop()
}

// onExit is called when systray is exiting
func (t *TrayUI) onExit() {
	// Stop health checker
	if t.healthChecker != nil {
		t.healthChecker.Stop()
	}

	// Stop proxy if running
	if t.manager.IsRunning() {
		t.manager.Stop()
	}
}

// buildMenu creates the tray menu
func (t *TrayUI) buildMenu() {
	// Status
	t.mStatus = systray.AddMenuItem("🔴 已停止", "当前状态")
	t.mStatus.Disable()

	systray.AddSeparator()

	// Control buttons
	t.mStart = systray.AddMenuItem("启动所有代理", "启动 sing-box")
	t.mStop = systray.AddMenuItem("停止所有代理", "停止 sing-box")
	t.mStop.Disable()
	t.mRestart = systray.AddMenuItem("重启所有代理", "重启 sing-box")
	t.mRestart.Disable()

	systray.AddSeparator()

	// Proxy list with health status
	t.mProxies = systray.AddMenuItem("代理列表", "显示配置的代理及健康状态")
	t.buildProxyList()

	systray.AddSeparator()

	// Health check controls
	if t.config.HealthCheck.Enabled {
		interval := t.config.HealthCheck.IntervalSeconds
		t.mHealthCheck = systray.AddMenuItem(
			fmt.Sprintf("健康检查: 已启用 (每%d秒)", interval),
			"健康检查状态",
		)
		t.mHealthCheck.Disable()

		t.mCheckNow = systray.AddMenuItem("立即检查所有节点", "手动触发健康检查")
	} else {
		t.mHealthCheck = systray.AddMenuItem("健康检查: 已禁用", "在配置文件中启用")
		t.mHealthCheck.Disable()
	}

	// DNS flush controls
	if t.config.DNS.FlushOnFailure {
		t.mDNSFlush = systray.AddMenuItem("DNS刷新: 已启用", "失败时自动刷新DNS")
		t.mDNSFlush.Disable()
	} else {
		t.mDNSFlush = systray.AddMenuItem("DNS刷新: 已禁用", "在配置文件中启用")
		t.mDNSFlush.Disable()
	}

	t.mFlushNow = systray.AddMenuItem("立即刷新DNS", "手动刷新系统DNS缓存")

	systray.AddSeparator()

	// Tools
	t.mOpenConfig = systray.AddMenuItem("打开配置文件", "使用默认编辑器打开")
	t.mViewLogs = systray.AddMenuItem("查看日志", "打开日志目录")

	systray.AddSeparator()

	// Quit
	t.mQuit = systray.AddMenuItem("退出", "退出 OneProxy")
}

// buildProxyList adds proxy items to submenu
func (t *TrayUI) buildProxyList() {
	for _, proxyConfig := range t.config.Proxies {
		if !proxyConfig.Enabled {
			continue
		}

		// Initial label (no health data yet)
		label := fmt.Sprintf("⚪ %s [:%d]", proxyConfig.Name, proxyConfig.LocalPort)
		item := t.mProxies.AddSubMenuItem(label, fmt.Sprintf("Port: %d", proxyConfig.LocalPort))
		item.Disable()

		// Store for later updates
		t.proxyMenuItems[proxyConfig.Name] = item
	}
}

// eventLoop handles menu events
func (t *TrayUI) eventLoop() {
	for {
		select {
		case <-t.mStart.ClickedCh:
			t.handleStart()

		case <-t.mStop.ClickedCh:
			t.handleStop()

		case <-t.mRestart.ClickedCh:
			t.handleRestart()

		case <-t.mCheckNow.ClickedCh:
			if t.mCheckNow != nil {
				t.handleCheckNow()
			}

		case <-t.mFlushNow.ClickedCh:
			t.handleFlushNow()

		case <-t.mOpenConfig.ClickedCh:
			t.handleOpenConfig()

		case <-t.mViewLogs.ClickedCh:
			t.handleViewLogs()

		case <-t.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// uiUpdateLoop periodically updates the UI with health status
func (t *TrayUI) uiUpdateLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		t.updateProxyHealthStatus()
	}
}

// updateProxyHealthStatus updates proxy menu items with health data
func (t *TrayUI) updateProxyHealthStatus() {
	if t.healthChecker == nil || !t.healthChecker.IsRunning() {
		return
	}

	results := t.healthChecker.GetAllResults()

	for proxyName, menuItem := range t.proxyMenuItems {
		result, exists := results[proxyName]
		if !exists {
			continue
		}

		// Format: ✓ JMS-Server1 [:10801] (45ms)
		var status string
		if result.IsHealthy {
			status = "✓"
			latency := result.Latency.Milliseconds()
			label := fmt.Sprintf("%s %s [:%d] (%dms)", status, proxyName, result.LocalPort, latency)
			menuItem.SetTitle(label)
		} else {
			status = "✗"
			if result.LastError != "" {
				label := fmt.Sprintf("%s %s [:%d] (失败)", status, proxyName, result.LocalPort)
				menuItem.SetTitle(label)
			} else {
				label := fmt.Sprintf("%s %s [:%d] (未检查)", status, proxyName, result.LocalPort)
				menuItem.SetTitle(label)
			}
		}
	}
}

// handleStart starts the proxy
func (t *TrayUI) handleStart() {
	if err := t.manager.Start(); err != nil {
		t.showError("启动失败", err)
		return
	}

	// Start health checker
	if t.healthChecker != nil && t.config.HealthCheck.Enabled {
		t.healthChecker.Start()
	}

	// Update UI
	t.setIcon(true)
	t.mStatus.SetTitle("🟢 运行中")
	t.mStart.Disable()
	t.mStop.Enable()
	t.mRestart.Enable()

	systray.SetTooltip("OneProxy - 运行中")
}

// handleStop stops the proxy
func (t *TrayUI) handleStop() {
	// Stop health checker
	if t.healthChecker != nil {
		t.healthChecker.Stop()
	}

	if err := t.manager.Stop(); err != nil {
		t.showError("停止失败", err)
		return
	}

	// Update UI
	t.setIcon(false)
	t.mStatus.SetTitle("🔴 已停止")
	t.mStart.Enable()
	t.mStop.Disable()
	t.mRestart.Disable()

	systray.SetTooltip("OneProxy - 已停止")
}

// handleRestart restarts the proxy
func (t *TrayUI) handleRestart() {
	if err := t.manager.Restart(); err != nil {
		t.showError("重启失败", err)
		return
	}

	// Restart health checker
	if t.healthChecker != nil && t.config.HealthCheck.Enabled {
		t.healthChecker.Stop()
		time.Sleep(100 * time.Millisecond)
		t.healthChecker.Start()
	}

	systray.SetTooltip("OneProxy - 已重启")
}

// handleCheckNow manually triggers health check
func (t *TrayUI) handleCheckNow() {
	if t.healthChecker == nil {
		t.showError("健康检查未启用", fmt.Errorf("请在配置文件中启用健康检查"))
		return
	}

	if !t.manager.IsRunning() {
		t.showError("代理未运行", fmt.Errorf("请先启动代理"))
		return
	}

	fmt.Println("手动触发健康检查...")
	go t.healthChecker.CheckAll()
}

// handleFlushNow manually flushes DNS
func (t *TrayUI) handleFlushNow() {
	if t.dnsFlusher == nil {
		t.showError("DNS刷新失败", fmt.Errorf("DNS flusher 未初始化"))
		return
	}

	if !t.dnsFlusher.CanFlush() {
		t.showError("刷新过于频繁", fmt.Errorf("请等待10秒后再试"))
		return
	}

	fmt.Println("手动刷新DNS...")
	if err := t.dnsFlusher.FlushAll(t.manager); err != nil {
		t.showError("DNS刷新失败", err)
	} else {
		fmt.Println("DNS刷新成功")
		systray.SetTooltip("OneProxy - DNS已刷新")
	}
}

// handleOpenConfig opens the config file
func (t *TrayUI) handleOpenConfig() {
	configPath := "config.json"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "configs/config.example.json"
	}

	absPath, _ := filepath.Abs(configPath)

	// Open with default editor (Windows)
	cmd := exec.Command("cmd", "/c", "start", "", absPath)
	if err := cmd.Start(); err != nil {
		t.showError("无法打开配置文件", err)
	}
}

// handleViewLogs opens the log directory
func (t *TrayUI) handleViewLogs() {
	logsPath, _ := filepath.Abs("logs")

	// Open directory (Windows)
	cmd := exec.Command("explorer", logsPath)
	if err := cmd.Start(); err != nil {
		t.showError("无法打开日志目录", err)
	}
}

// setIcon sets the tray icon based on status
func (t *TrayUI) setIcon(running bool) {
	if running {
		// Green icon (running)
		systray.SetIcon(getGreenIcon())
	} else {
		// Red icon (stopped)
		systray.SetIcon(getRedIcon())
	}
}

// showError displays an error (currently just prints, could use notifications)
func (t *TrayUI) showError(title string, err error) {
	msg := fmt.Sprintf("%s: %v", title, err)
	fmt.Println(msg)
	// TODO: Add Windows notification support
}

// Icon data (simple colored dots)
func getGreenIcon() []byte {
	// Simple green dot icon (16x16 ICO format)
	return []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10,
		0x00, 0x00, 0x01, 0x00, 0x20, 0x00, 0x68, 0x04,
		0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
	}
}

func getRedIcon() []byte {
	// Simple red dot icon (16x16 ICO format)
	return []byte{
		0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10,
		0x00, 0x00, 0x01, 0x00, 0x20, 0x00, 0x68, 0x04,
		0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
	}
}
