// OneProxy Tray — C++17 Qt6 + Go shared library
#include <QApplication>
#include <QCoreApplication>
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
#include <QClipboard>
#include <QDebug>
#include <QDesktopServices>
#include <QLibrary>
#include <QUrl>
#include <thread>
#include <functional>
#include "i18n.h"
#include "platform/platform.h"

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
    static QLibrary library;
    if (library.isLoaded()) return true;

    library.setFileName(QDir(QCoreApplication::applicationDirPath())
                            .filePath(Platform::sharedLibraryName()));
    if (!library.load()) {
        qCritical() << "Shared library load failed:" << library.errorString();
        return false;
    }

    #define L(fn,n) fn = reinterpret_cast<decltype(fn)>(library.resolve(n))
    L(pStart,"OneProxy_Start"); L(pStop,"OneProxy_Stop"); L(pRestart,"OneProxy_Restart");
    L(pStatus,"OneProxy_Status"); L(pCheck,"OneProxy_HealthCheck"); L(pFlush,"OneProxy_FlushDNS");
    L(pSelect,"OneProxy_SelectProxy"); L(pExport,"OneProxy_ExportConfig"); L(pImport,"OneProxy_ImportConfig"); L(pFree,"OneProxy_FreeString");
    #undef L
    const bool resolved = pStart && pStop && pRestart && pStatus && pCheck &&
                          pFlush && pSelect && pExport && pImport && pFree;
    if (!resolved) {
        qCritical() << "Shared library exports are incomplete";
        library.unload();
    }
    return resolved;
}

QString callFree(char* p) {
    if (!p) return {};
    QString r = QString::fromUtf8(p);
    if (pFree) pFree(p);
    return r;
}

// ─── Icons ──────────────────────────────────────────
#include <QPainter>
#include <QPixmap>
#include <QColor>
static QIcon icoGreen, icoYellow, icoRed;

static QIcon loadIcon(const QString &name) {
    QIcon ic(QDir(QCoreApplication::applicationDirPath()).filePath(name));
    if (!ic.isNull()) return ic;
    ic = QIcon(QStringLiteral(":/icons/") + name);
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
        else if (ok == 0)     { tray->setIcon(icoRed);    tray->setToolTip(QString("OneProxy %1/%2 all down").arg(ok).arg(total)); }
    }

    // Called only on QMenu::aboutToShow — rebuilds items right before display.
    void rebuildMenu() {
        bool running; int unifiedPort, ok, total; QJsonArray proxies; QString active;
        if (!fetchStatus(running, unifiedPort, proxies, ok, total, active)) return;

        auto s = getStrings();
        menu->clear();

        // Status — always at top
        menu->addAction(running ? s.running : s.stopped)->setEnabled(false);
        menu->addSeparator();

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
                ? QString("127.0.0.1:%1  auto  waiting...").arg(unifiedPort)
                : QString("127.0.0.1:%1  auto ◀ %2  %3ms").arg(unifiedPort).arg(active).arg(activeLat);
            menu->addAction(line);
            menu->addSeparator();
        }

        // Node list
        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            QString name = px["name"].toString();
            bool h = px["is_healthy"].toBool();
            int port = px["port"].toInt();          // local port
            int lat = px["latency_ms"].toInt();
            QString typ = px["type"].toString();
            QString srv = px["server"].toString();
            int srvPort = px["server_port"].toInt();
            bool isActive = (unifiedPort > 0 && h && name == active);

            QString proto = (typ == "shadowsocks") ? "SS" : (typ == "vmess") ? "VM" : (typ == "vless") ? "VL" : typ.left(3);
            QString dot = isActive ? "●" : (h ? "○" : "✗");
            QString label = h
                ? QString("  %1  %2:%3 → :%4  %5  %6ms")
                    .arg(dot).arg(name).arg(srvPort).arg(port).arg(proto).arg(lat)
                : QString("  %1  %2:%3 → :%4  %5  %6")
                    .arg(dot).arg(name).arg(srvPort).arg(port).arg(proto).arg(s.timeout);

            auto* a = menu->addAction(label);
            if (h && unifiedPort > 0 && !isActive) {
                connect(a, &QAction::triggered, this, [this, name]() { selectProxy(name); });
            } else {
                a->setEnabled(false);
            }
        }
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

        // Routing mode submenu
        auto *routeMenu = menu->addMenu(s.routingMode);
        routeMenu->setEnabled(Platform::routingModeSupported());
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
        autoAction->setEnabled(Platform::autoStartSupported());
        connect(autoAction, &QAction::toggled, this, [this](bool on) { setAutoStart(on); });

        menu->addSeparator();
        menu->addAction(s.openConfig, this, [this]() { doOpenConfig(); });
        menu->addAction(s.exportConfig, this, &OneProxyTray::doExportConfig);
        menu->addAction(s.importClipboard, this, &OneProxyTray::doImportClipboard);
        menu->addAction(s.importBackup, this, &OneProxyTray::doImportBackup);
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
            QDesktopServices::openUrl(QUrl::fromLocalFile(path));
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

    void doImportClipboard() {
        QClipboard *clipboard = QApplication::clipboard();
        QString text = clipboard->text().trimmed();
        if (text.isEmpty()) {
            tray->showMessage("OneProxy", "Clipboard is empty", QSystemTrayIcon::Warning, 3000);
            return;
        }
        if (!text.startsWith("http://") && !text.startsWith("https://") &&
            !text.startsWith("ss://") && !text.startsWith("vmess://") && !text.startsWith("vless://")) {
            tray->showMessage("OneProxy", "Clipboard is not a valid subscription URL", QSystemTrayIcon::Warning, 3000);
            return;
        }
        auto err = callFree(pImport((char*)text.toUtf8().constData()));
        if (!err.isEmpty()) {
            tray->showMessage("OneProxy", "Subscription import failed: " + err, QSystemTrayIcon::Critical, 5000);
            return;
        }
        tray->showMessage("OneProxy", "Subscription imported, restarting...", QSystemTrayIcon::Information, 2000);
    }

    void doImportBackup() {
        QString path = QFileDialog::getOpenFileName(nullptr, "Import Backup",
            QDir::homePath(), "Config Backup (*.*)");
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
    }

    static bool isAutoStart() {
        return Platform::autoStartEnabled();
    }

    static void setAutoStart(bool on) {
        Platform::setAutoStart(on);
    }

    QString routingMode() {
        return Platform::routingMode();
    }

    void setRoutingMode(const QString &m) {
        if (!Platform::setRoutingMode(m)) return;
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
    auto *t = new OneProxyTray;
    Platform::initializeTrayRecovery(t->tray);
    return app.exec();
}

#include "main.moc"
