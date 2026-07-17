// OneProxy Tray — C++17 Qt6 + Go DLL
// Build: cmake + nmake with MSVC 2022
#include <QApplication>
#include <QSystemTrayIcon>
#include <QMenu>
#include <QAction>
#include <QActionGroup>
#include <QTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QFile>
#include <QDir>
#include <QFileDialog>
#include <QDebug>
#include <thread>
#include <functional>
#include <windows.h>
#include <wininet.h>
#pragma comment(lib, "wininet.lib")
#include "i18n.h"

// ─── DLL bindings ──────────────────────────────────
typedef char* (*PFN_Start)(char*);
typedef char* (*PFN_Stop)();
typedef char* (*PFN_Restart)();
typedef char* (*PFN_Status)();
typedef char* (*PFN_Check)();
typedef char* (*PFN_Flush)();
typedef char* (*PFN_Select)(char*);
typedef char* (*PFN_Export)();
typedef char* (*PFN_Import)(char*);
typedef void  (*PFN_Free)(char*);

static PFN_Start  pStart;
static PFN_Stop   pStop;
static PFN_Restart pRestart;
static PFN_Status pStatus;
static PFN_Check  pCheck;
static PFN_Flush  pFlush;
static PFN_Select pSelect;
static PFN_Export pExport;
static PFN_Import pImport;
static PFN_Free   pFree;

bool loadDLL() {
    static HMODULE dll = nullptr;
    if (dll) return true;
    dll = LoadLibraryW(L"oneproxy.dll");
    if (!dll) {
        wchar_t exe[1024]; GetModuleFileNameW(nullptr, exe, 1024);
        std::wstring p(exe); p = p.substr(0, p.find_last_of(L"\\/"));
        SetCurrentDirectoryW(p.c_str());
        dll = LoadLibraryW(L"oneproxy.dll");
    }
    if (!dll) { qCritical() << "DLL load failed" << GetLastError(); return false; }
    #define L(fn,n) fn = (decltype(fn))GetProcAddress(dll, n)
    L(pStart,"OneProxy_Start"); L(pStop,"OneProxy_Stop"); L(pRestart,"OneProxy_Restart");
    L(pStatus,"OneProxy_Status"); L(pCheck,"OneProxy_HealthCheck"); L(pFlush,"OneProxy_FlushDNS");
    L(pSelect,"OneProxy_SelectProxy"); L(pExport,"OneProxy_ExportConfig"); L(pImport,"OneProxy_ImportConfig"); L(pFree,"OneProxy_FreeString");
    #undef L
    return true;
}

QString callFree(char* p) {
    if (!p) return {};
    QString r = QString::fromUtf8(p);
    if (pFree) pFree(p);
    return r;
}

// ─── TaskbarCreated: re-show tray icon after explorer.exe restarts ──
// Windows broadcasts the registered "TaskbarCreated" message when the shell
// restarts. A hidden message-only window listens for it and re-adds the icon.
static UINT WM_TASKBARCREATED = 0;
static QSystemTrayIcon *g_tray = nullptr;

static LRESULT CALLBACK TrayWndProc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    if (msg == WM_TASKBARCREATED && g_tray) {
        g_tray->hide();
        g_tray->show();
    }
    return DefWindowProcW(hwnd, msg, wp, lp);
}

static void createTrayWindow() {
    WM_TASKBARCREATED = RegisterWindowMessageW(L"TaskbarCreated");

    WNDCLASSEXW wc = {};
    wc.cbSize = sizeof(wc);
    wc.lpfnWndProc = TrayWndProc;
    wc.hInstance = GetModuleHandleW(nullptr);
    wc.lpszClassName = L"OneProxyTrayWindow";
    RegisterClassExW(&wc);

    CreateWindowExW(0, L"OneProxyTrayWindow", L"", 0, 0, 0, 0, 0,
                    HWND_MESSAGE, nullptr, GetModuleHandleW(nullptr), nullptr);
}

// ─── Icons ──────────────────────────────────────────
#include <QPainter>
#include <QPixmap>
#include <QColor>
static QIcon icoGreen, icoYellow, icoRed;

static QString exeDir() {
    wchar_t b[MAX_PATH]; GetModuleFileNameW(nullptr, b, MAX_PATH);
    QString path = QString::fromWCharArray(b);
    return path.left(path.lastIndexOf('\\'));
}

static QIcon loadIcon(const QString &name) {
    QIcon ic(exeDir() + "\\" + name);
    if (!ic.isNull()) return ic;
    QPixmap px(64,64); px.fill(Qt::transparent);
    QPainter pr(&px); pr.setRenderHint(QPainter::Antialiasing);
    QColor c = name.startsWith("green") ? QColor(0x2E,0x8B,0x57)
             : name.startsWith("yellow") ? QColor(0xE0,0xA0,0x00) : QColor(0xC0,0x39,0x2B);
    pr.setBrush(c); pr.setPen(Qt::NoPen); pr.drawEllipse(4,4,56,56); pr.end();
    return QIcon(px);
}

// ─── Tray ───────────────────────────────────────────
class OneProxyTray : public QObject {
    Q_OBJECT
public:
    QSystemTrayIcon *tray;
    QMenu *menu;
    QTimer timer;

    OneProxyTray() {
        tray = new QSystemTrayIcon(this);
        menu = new QMenu();
        tray->setContextMenu(menu);
        // Rebuild the menu only when it's about to be shown, so an open
        // menu is never torn out from under the user by the refresh timer.
        connect(menu, &QMenu::aboutToShow, this, &OneProxyTray::rebuildMenu);

        tray->setIcon(icoRed);
        tray->setToolTip("OneProxy");
        tray->show();

        // Timer only refreshes the icon/tooltip, never the menu.
        connect(&timer, &QTimer::timeout, this, &OneProxyTray::tick);
        timer.start(5000);
        QTimer::singleShot(600, this, &OneProxyTray::autoStart);
    }

private:
    void autoStart() {
        auto err = callFree(pStart((char*)"config.json"));
        if (!err.isEmpty()) {
            qDebug() << "Start failed:" << err;
            tray->showMessage("OneProxy", "Start failed: " + err, QSystemTrayIcon::Critical, 5000);
            tick();
            return;
        }
        qDebug() << "OneProxy: started";
        // Give sing-box 3s to bind, then health-check off-thread and refresh.
        QTimer::singleShot(3000, this, [this]() {
            runAsync([]() { callFree(pCheck()); }, 8000);
        });
    }

    // Parse status JSON; returns false if unavailable.
    bool fetchStatus(bool &running, int &unifiedPort, QJsonArray &proxies,
                     int &ok, int &total, QString &active) {
        auto json = callFree(pStatus());
        if (json.isEmpty()) return false;
        auto obj = QJsonDocument::fromJson(json.toUtf8()).object();
        running = obj["running"].toBool();
        unifiedPort = obj["unified_port"].toInt();
        proxies = obj["proxies"].toArray();

        ok = 0; total = 0; active.clear();
        int minLat = 999999;
        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            ++total;
            if (px["is_healthy"].toBool()) {
                ++ok;
                int lat = px["latency_ms"].toInt();
                if (lat < minLat) { minLat = lat; active = px["name"].toString(); }
            }
        }
        return true;
    }

    // Called by the 5s timer — only touches the icon/tooltip, never the menu.
    void tick() {
        bool running; int unifiedPort, ok, total; QJsonArray proxies; QString active;
        if (!fetchStatus(running, unifiedPort, proxies, ok, total, active)) return;

        if (!running)         { tray->setIcon(icoRed);    tray->setToolTip("OneProxy — stopped"); }
        else if (total == 0)  { tray->setIcon(icoRed);    tray->setToolTip("OneProxy — no proxies"); }
        else if (ok == total) { tray->setIcon(icoGreen);  tray->setToolTip(QString("OneProxy %1/%2 OK").arg(ok).arg(total)); }
        else if (ok > 0)      { tray->setIcon(icoYellow); tray->setToolTip(QString("OneProxy %1/%2").arg(ok).arg(total)); }
        else                  { tray->setIcon(icoYellow); tray->setToolTip(QString("OneProxy %1 proxies, checking...").arg(total)); }
    }

    // Called only on QMenu::aboutToShow — rebuilds items right before display.
    void rebuildMenu() {
        bool running; int unifiedPort, ok, total; QJsonArray proxies; QString active;
        if (!fetchStatus(running, unifiedPort, proxies, ok, total, active)) return;

        auto s = getStrings();
        menu->clear();

        // Unified proxy: show the address and active node at the top
        if (unifiedPort > 0 && running) {
            int activeLat = 0;
            for (const auto& p : proxies) {
                auto px = p.toObject();
                if (px["name"].toString() == active && px["is_healthy"].toBool()) {
                    activeLat = px["latency_ms"].toInt();
                    break;
                }
            }
            QString line = active.isEmpty()
                ? QString("127.0.0.1:%1 — waiting...").arg(unifiedPort)
                : QString("127.0.0.1:%1 ◀ %2 %3ms").arg(unifiedPort).arg(active).arg(activeLat);
            menu->addAction(line)->setEnabled(false);
            menu->addSeparator();
        }

        // Node list
        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            QString name = px["name"].toString();
            bool h = px["is_healthy"].toBool();
            int port = px["port"].toInt();
            int lat = px["latency_ms"].toInt();
            bool isActive = (unifiedPort > 0 && h && name == active);

            QString dot = isActive ? "●" : (h ? "○" : "✗");
            QString label = h
                ? QString("  %1 %2  :%3  %4ms").arg(dot).arg(name, -16).arg(port, 5).arg(lat)
                : QString("  %1 %2  :%3  %4").arg(dot).arg(name, -16).arg(port, 5).arg(s.timeout);

            auto* a = menu->addAction(label);
            if (h && unifiedPort > 0 && !isActive) {
                connect(a, &QAction::triggered, this, [this, name]() { selectProxy(name); });
            } else {
                a->setEnabled(false);
            }
        }

        menu->addSeparator();
        menu->addAction(running ? s.running : s.stopped)->setEnabled(false);
        menu->addSeparator();

        if (running) {
            menu->addAction(s.stop, this, &OneProxyTray::doStop);
            menu->addAction(s.restart, this, &OneProxyTray::doRestart);
        } else {
            menu->addAction(s.start, this, &OneProxyTray::doStart);
        }
        menu->addSeparator();
        menu->addAction(s.check, this, &OneProxyTray::doCheck);
        menu->addAction(s.flushDNS, this, &OneProxyTray::doFlush);
        menu->addSeparator();

        // System proxy toggle
        QAction *sysProxyAction = menu->addAction(s.systemProxy);
        sysProxyAction->setCheckable(true);
        sysProxyAction->setChecked(isSystemProxy());
        connect(sysProxyAction, &QAction::toggled, this, [this](bool on) { setSystemProxy(on); });

        // Routing mode submenu
        auto *routeMenu = menu->addMenu(s.routingMode);
        auto *routeGroup = new QActionGroup(this);
        routeGroup->setExclusive(true);

        auto *actGlobal = routeMenu->addAction(s.modeGlobal);
        actGlobal->setCheckable(true); routeGroup->addAction(actGlobal);
        auto *actRule  = routeMenu->addAction(s.modeRule);
        actRule->setCheckable(true);  routeGroup->addAction(actRule);
        auto *actDirect = routeMenu->addAction(s.modeDirect);
        actDirect->setCheckable(true); routeGroup->addAction(actDirect);

        QString curMode = routingMode();
        (curMode == "rule" ? actRule : curMode == "direct" ? actDirect : actGlobal)->setChecked(true);

        connect(actGlobal, &QAction::triggered, this, [this]() { setRoutingMode("global"); });
        connect(actRule,  &QAction::triggered, this, [this]() { setRoutingMode("rule"); });
        connect(actDirect,&QAction::triggered, this, [this]() { setRoutingMode("direct"); });

        menu->addSeparator();

        // Auto-start toggle
        QAction *autoAction = menu->addAction(s.autoStart);
        autoAction->setCheckable(true);
        autoAction->setChecked(isAutoStart());
        connect(autoAction, &QAction::toggled, this, [this](bool on) { setAutoStart(on); });

        menu->addSeparator();
        menu->addAction(s.openConfig, this, [this]() { doOpenConfig(); });
        menu->addAction(s.exportConfig, this, &OneProxyTray::doExportConfig);
        menu->addAction(s.importConfig, this, &OneProxyTray::doImportConfig);
        menu->addSeparator();
        menu->addAction(s.quit, this, &OneProxyTray::doQuit);
    }

    // Run a blocking DLL call off the UI thread, then refresh on the UI thread.
    // QMetaObject::invokeMethod with QueuedConnection is the safe cross-thread
    // way to schedule tick() back on the Qt main thread.
    void runAsync(std::function<void()> work, int refreshDelayMs) {
        std::thread([this, work, refreshDelayMs]() {
            work();
            QMetaObject::invokeMethod(this, [this, refreshDelayMs]() {
                QTimer::singleShot(refreshDelayMs, this, &OneProxyTray::tick);
            }, Qt::QueuedConnection);
        }).detach();
    }

    void selectProxy(const QString &name) {
        QByteArray raw = name.toUtf8();
        runAsync([raw]() { callFree(pSelect(const_cast<char*>(raw.constData()))); }, 500);
    }

    void doStart()    { callFree(pStart((char*)"config.json")); tick(); }
    void doStop()     { callFree(pStop()); tick(); }
    void doRestart()  {
        callFree(pRestart());
        QTimer::singleShot(3000, this, [this]() {
            runAsync([]() { callFree(pCheck()); }, 8000);
        });
    }
    void doCheck()    { runAsync([]() { callFree(pCheck()); }, 0); }
    void doFlush()    { callFree(pFlush()); QTimer::singleShot(2000, this, &OneProxyTray::tick); }
    void doQuit()     { callFree(pStop()); tray->hide(); QApplication::quit(); }

    void doOpenConfig() {
        QString path = QDir::homePath() + "/.oneproxy/config.json";
        if (!QFile::exists(path)) {
            path = QDir::currentPath() + "/config.json";
            if (!QFile::exists(path)) path = "";
        }
        if (!path.isEmpty()) {
            ShellExecuteW(nullptr, L"open", L"notepad.exe",
                          (L"\"" + path.toStdWString() + L"\"").c_str(),
                          nullptr, SW_SHOW);
        }
    }

    void doExportConfig() {
        QString path = QFileDialog::getSaveFileName(nullptr, "Export Config",
            QDir::homePath() + "/oneproxy-config.json", "JSON (*.json)");
        if (path.isEmpty()) return;
        auto b64 = callFree(pExport());
        if (b64.isEmpty()) { tray->showMessage("OneProxy", "Export failed", QSystemTrayIcon::Critical, 3000); return; }
        QFile f(path);
        if (f.open(QIODevice::WriteOnly | QIODevice::Text)) {
            f.write(b64.toUtf8()); f.close();
            tray->showMessage("OneProxy", "Config exported", QSystemTrayIcon::Information, 2000);
        }
    }

    void doImportConfig() {
        QString path = QFileDialog::getOpenFileName(nullptr, "Import Config",
            QDir::homePath(), "JSON (*.json)");
        if (path.isEmpty()) return;
        QFile f(path);
        if (!f.open(QIODevice::ReadOnly | QIODevice::Text)) return;
        auto b64 = QString::fromUtf8(f.readAll()).trimmed(); f.close();
        auto err = callFree(pImport((char*)b64.toUtf8().constData()));
        if (!err.isEmpty()) {
            tray->showMessage("OneProxy", "Import failed: " + err, QSystemTrayIcon::Critical, 5000);
            return;
        }
        tray->showMessage("OneProxy", "Config imported, restarting...", QSystemTrayIcon::Information, 2000);
        QTimer::singleShot(1000, this, [this]() { doRestart(); });
    }

    static bool isAutoStart() {
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_CURRENT_USER,
            L"Software\\Microsoft\\Windows\\CurrentVersion\\Run", 0, KEY_READ, &hKey) != ERROR_SUCCESS)
            return false;
        wchar_t val[1024] = {};
        DWORD sz = sizeof(val);
        auto r = RegQueryValueExW(hKey, L"OneProxy", nullptr, nullptr, (LPBYTE)val, &sz);
        RegCloseKey(hKey);
        return r == ERROR_SUCCESS;
    }

    static void setAutoStart(bool on) {
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_CURRENT_USER,
            L"Software\\Microsoft\\Windows\\CurrentVersion\\Run", 0, KEY_SET_VALUE, &hKey) != ERROR_SUCCESS)
            return;
        if (on) {
            wchar_t exe[MAX_PATH];
            GetModuleFileNameW(nullptr, exe, MAX_PATH);
            RegSetValueExW(hKey, L"OneProxy", 0, REG_SZ, (BYTE*)exe,
                           (DWORD)((wcslen(exe) + 1) * sizeof(wchar_t)));
        } else {
            RegDeleteValueW(hKey, L"OneProxy");
        }
        RegCloseKey(hKey);
    }

    static bool isSystemProxy() {
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_CURRENT_USER,
            L"Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", 0, KEY_READ, &hKey) != ERROR_SUCCESS)
            return false;
        DWORD val = 0, sz = sizeof(val);
        RegQueryValueExW(hKey, L"ProxyEnable", nullptr, nullptr, (LPBYTE)&val, &sz);
        RegCloseKey(hKey);
        return val != 0;
    }

    static void setSystemProxy(bool on) {
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_CURRENT_USER,
            L"Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings", 0, KEY_SET_VALUE, &hKey) != ERROR_SUCCESS)
            return;
        if (on) {
            DWORD en = 1;
            RegSetValueExW(hKey, L"ProxyEnable", 0, REG_DWORD, (BYTE*)&en, sizeof(en));
            auto server = L"socks=127.0.0.1:1080";
            RegSetValueExW(hKey, L"ProxyServer", 0, REG_SZ, (BYTE*)server, (DWORD)((wcslen(server)+1)*sizeof(wchar_t)));
            auto override = L"<local>";
            RegSetValueExW(hKey, L"ProxyOverride", 0, REG_SZ, (BYTE*)override, (DWORD)((wcslen(override)+1)*sizeof(wchar_t)));
        } else {
            DWORD en = 0;
            RegSetValueExW(hKey, L"ProxyEnable", 0, REG_DWORD, (BYTE*)&en, sizeof(en));
        }
        RegCloseKey(hKey);
        // Notify Windows to pick up the change immediately
        InternetSetOptionW(nullptr, 39, nullptr, 0);  // INTERNET_OPTION_SETTINGS_CHANGED
        InternetSetOptionW(nullptr, 37, nullptr, 0);  // INTERNET_OPTION_REFRESH
    }

    // Current routing mode — persisted in a tiny registry string (no DLL needed)
    QString routingMode() {
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_CURRENT_USER,
            L"Software\\OneProxy", 0, KEY_READ, &hKey) != ERROR_SUCCESS)
            return "global";
        wchar_t val[32] = {};
        DWORD sz = sizeof(val);
        RegQueryValueExW(hKey, L"RouteMode", nullptr, nullptr, (LPBYTE)val, &sz);
        RegCloseKey(hKey);
        return QString::fromWCharArray(val).isEmpty() ? "global" : QString::fromWCharArray(val);
    }

    void setRoutingMode(const QString &m) {
        HKEY hKey;
        RegCreateKeyExW(HKEY_CURRENT_USER, L"Software\\OneProxy", 0, nullptr, 0, KEY_SET_VALUE, nullptr, &hKey, nullptr);
        auto s = m.toStdWString();
        RegSetValueExW(hKey, L"RouteMode", 0, REG_SZ, (BYTE*)s.c_str(), (DWORD)((s.size()+1)*sizeof(wchar_t)));
        RegCloseKey(hKey);
        // Restart to apply new route
        if (tray) {  // bit of delay: stop → restart
            callFree(pStop());
            QTimer::singleShot(500, this, [this]() { doStart(); });
        }
    }

};

int main(int argc, char *argv[]) {
    QApplication app(argc, argv);
    app.setQuitOnLastWindowClosed(false);

    // Load icons and DLL BEFORE constructing the tray (ctor uses both)
    qDebug() << "loading icons...";
    icoGreen  = loadIcon("green.ico");  qDebug() << "  green ok";
    icoYellow = loadIcon("yellow.ico"); qDebug() << "  yellow ok";
    icoRed    = loadIcon("red.ico");    qDebug() << "  red ok";
    if (!loadDLL()) { qCritical() << "DLL failed"; return 1; }
    qDebug() << "DLL OK";
    if (!QFile::exists("config.json")) { qCritical() << "no config.json"; return 1; }

    createTrayWindow();
    auto *t = new OneProxyTray;
    g_tray = t->tray;
    return app.exec();
}

#include "main.moc"
