// OneProxy Tray — Native C++ Qt6 + Go DLL
// Build: mkdir build && cd build && cmake .. -G "MinGW Makefiles" && make
#include <QApplication>
#include <QSystemTrayIcon>
#include <QMenu>
#include <QAction>
#include <QPainter>
#include <QPixmap>
#include <QTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QFile>
#include <QDir>
#include <QMessageBox>
#include "i18n.h"
#include <QColor>
#include <QDebug>
#include <QTextStream>
#include <thread>
#include <windows.h>

// ─── DLL bindings ──────────────────────────────────
typedef char* (*PFN_Start)(char*);
typedef char* (*PFN_Stop)();
typedef char* (*PFN_Restart)();
typedef char* (*PFN_Status)();
typedef char* (*PFN_Check)();
typedef char* (*PFN_Flush)();
typedef char* (*PFN_Version)();
typedef void  (*PFN_Free)(char*);

static PFN_Start   pStart;
static PFN_Stop    pStop;
static PFN_Restart pRestart;
static PFN_Status  pStatus;
static PFN_Check   pCheck;
static PFN_Flush   pFlush;
static PFN_Version pVersion;
static PFN_Free    pFree;

static HMODULE dll = nullptr;

bool loadDLL() {
    if (dll) return true;
    // Try exe dir first, then parent dir
    dll = LoadLibraryW(L"oneproxy.dll");
    if (!dll) {
        wchar_t exe[1024];
        GetModuleFileNameW(nullptr, exe, 1024);
        std::wstring path(exe);
        path = path.substr(0, path.find_last_of(L"\\/"));
        SetCurrentDirectoryW(path.c_str());
        dll = LoadLibraryW(L"oneproxy.dll");
    }
    if (!dll) return false;

    #define LOAD(fn, name) fn = (decltype(fn))GetProcAddress(dll, name)
    LOAD(pStart,   "OneProxy_Start");
    LOAD(pStop,    "OneProxy_Stop");
    LOAD(pRestart, "OneProxy_Restart");
    LOAD(pStatus,  "OneProxy_Status");
    LOAD(pCheck,   "OneProxy_HealthCheck");
    LOAD(pFlush,   "OneProxy_FlushDNS");
    LOAD(pVersion, "OneProxy_GetVersion");
    LOAD(pFree,    "OneProxy_FreeString");
    #undef LOAD
    return true;
}

QString callFree(char* p) {
    if (!p) return {};
    QString r = QString::fromUtf8(p);
    if (pFree) pFree(p);
    return r;
}

// ─── Icons (loaded from .ico files next to the exe) ──
static QIcon icoGreen;
static QIcon icoYellow;
static QIcon icoRed;

static QString exeDir() {
    wchar_t buf[MAX_PATH];
    GetModuleFileNameW(nullptr, buf, MAX_PATH);
    QString p = QString::fromWCharArray(buf);
    return p.left(p.lastIndexOf('\\'));
}

static QIcon loadIcon(const QString &name) {
    QString path = exeDir() + "\\" + name;
    QIcon ic(path);
    if (ic.isNull()) {
        // fallback: draw a colored circle so we never crash on missing file
        QPixmap px(64, 64);
        px.fill(Qt::transparent);
        QPainter p(&px);
        p.setRenderHint(QPainter::Antialiasing);
        QColor c = name.startsWith("green") ? QColor(0x2E, 0x8B, 0x57)
                 : name.startsWith("yellow") ? QColor(0xE0, 0xA0, 0x00)
                 : QColor(0xC0, 0x39, 0x2B);
        p.setBrush(c);
        p.setPen(Qt::NoPen);
        p.drawEllipse(4, 4, 56, 56);
        p.end();
        return QIcon(px);
    }
    return ic;
}

// ─── Main tray class ─────────────────────────────────
class OneProxyTray : public QObject {
    Q_OBJECT
public:
    OneProxyTray() {
        tray.setIcon(icoRed);
        tray.setToolTip("OneProxy — 已停止");
        tray.show();

        connect(&timer, &QTimer::timeout, this, &OneProxyTray::tick);
        timer.start(5000);

        // auto-start after tray is visible
        QTimer::singleShot(600, this, &OneProxyTray::autoStart);
    }

private Q_SLOTS:
    void autoStart() {
        auto err = callFree(pStart((char*)"config.json"));
        if (!err.isEmpty()) {
            qDebug() << "Start failed:" << err;
            tick();
        } else {
            qDebug() << "OneProxy: started";
            QTimer::singleShot(3000, this, [this]() {
                qDebug() << "Running health check (background)...";
                std::thread([this]() { callFree(pCheck()); }).detach();
                QTimer::singleShot(8000, this, &OneProxyTray::tick);
            });
        }
    }
    void tick() {
        auto json = callFree(pStatus());
        if (json.isEmpty()) return;

        auto doc = QJsonDocument::fromJson(json.toUtf8());
        auto obj = doc.object();
        bool running = obj["running"].toBool();
        auto mode = obj["mode"].toString();
        bool unified = (mode == "unified");
        int unifiedPort = obj["unified_port"].toInt();
        auto proxies = obj["proxies"].toArray();

        int total = 0, ok = 0, lowestLat = 999999;
        QString activeProxy;
        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            total++;
            if (px["is_healthy"].toBool()) {
                ok++;
                int lat = px["latency_ms"].toInt();
                if (lat < lowestLat) { lowestLat = lat; activeProxy = px["name"].toString(); }
            }
        }

        auto s = getStrings();
        if (!running) {
            tray.setIcon(icoRed);
            tray.setToolTip(QString("OneProxy — %1").arg(s.stopped.mid(2)));
        } else if (total == 0) {
            tray.setIcon(icoRed);
            tray.setToolTip("OneProxy — No proxies");
        } else if (ok == total) {
            tray.setIcon(icoGreen);
            tray.setToolTip(unified ? QString("OneProxy :%1 → %2").arg(unifiedPort).arg(activeProxy)
                                    : QString("OneProxy %1/%2 ✓").arg(ok).arg(total));
        } else if (ok > 0) {
            tray.setIcon(icoYellow);
            tray.setToolTip(unified ? QString("OneProxy :%1 → %2 (%3/%4)").arg(unifiedPort).arg(activeProxy).arg(ok).arg(total)
                                    : QString("OneProxy %1/%2").arg(ok).arg(total));
        } else {
            tray.setIcon(icoRed);
            tray.setToolTip(QString("OneProxy — All %1").arg(s.timeout));
        }

        // Rebuild menu
        auto* menu = new QMenu();

        // Unified mode header
        if (unified && running) {
            auto* a = menu->addAction(QString("Unified :%1 → %2 (%3ms)").arg(unifiedPort).arg(activeProxy).arg(lowestLat));
            a->setEnabled(false);
            menu->addSeparator();
        }

        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            QString name = px["name"].toString();
            int port = unified ? unifiedPort : px["port"].toInt();
            bool h = px["is_healthy"].toBool();
            qint64 lat = px["latency_ms"].toInt();
            QString mark = (unified && h && (name == activeProxy)) ? " ▶" : "";
            QString label = h
                ? QString("  ✓%1 %2  :%3  %4ms").arg(mark).arg(name, -16).arg(port, 5).arg(lat)
                : QString("  ✗ %1  :%2  %3").arg(name, -16).arg(port, 5).arg(s.timeout);
            auto* a = menu->addAction(label);
            a->setEnabled(false);
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
        menu->addAction(s.quit, this, &OneProxyTray::doQuit);

        auto* old = tray.contextMenu();
        tray.setContextMenu(menu);
        delete old;
    }

    void doStart()  { callFree(pStart((char*)"config.json")); tick(); }
    void doStop()   { callFree(pStop()); tick(); }
    void doRestart() {
        callFree(pRestart());
        QTimer::singleShot(3000, this, [this]() {
            std::thread([this]() { callFree(pCheck()); }).detach();
            QTimer::singleShot(8000, this, &OneProxyTray::tick);
        });
    }
    void doCheck()  {
        auto s = getStrings();
        tray.showMessage(s.checkingTitle, s.checking, QSystemTrayIcon::Information, 2000);
        std::thread([this]() {
            callFree(pCheck());
            QTimer::singleShot(0, this, &OneProxyTray::tick);
        }).detach();
    }
    void doFlush()  {
        auto s = getStrings();
        tray.showMessage(s.flushingTitle, s.flushing, QSystemTrayIcon::Information, 2000);
        callFree(pFlush());
        QTimer::singleShot(2000, this, &OneProxyTray::tick);
    }
    void doQuit()   { callFree(pStop()); tray.hide(); QApplication::quit(); }

private:
    QSystemTrayIcon tray;
    QTimer timer;
};

// ─── main ────────────────────────────────────────────
int main(int argc, char *argv[]) {
    QApplication app(argc, argv);
    app.setQuitOnLastWindowClosed(false);

    // Load icons AFTER QApplication exists
    qDebug() << "loading icons...";
    icoGreen  = loadIcon("green.ico");
    qDebug() << "  green ok";
    icoYellow = loadIcon("yellow.ico");
    qDebug() << "  yellow ok";
    icoRed    = loadIcon("red.ico");
    qDebug() << "  red ok";

    // Write startup log
    {
        wchar_t exePath[MAX_PATH];
        GetModuleFileNameW(nullptr, exePath, MAX_PATH);
        QString exeDir = QString::fromWCharArray(exePath);
        exeDir = exeDir.left(exeDir.lastIndexOf('\\'));
        QString logPath = exeDir + "\\oneproxy-tray.log";
        QFile logFile(logPath);
        if (logFile.open(QIODevice::WriteOnly | QIODevice::Text)) {
            QTextStream ts(&logFile);
            ts << "exe: " << QString::fromWCharArray(exePath) << "\n";
            ts << "dir: " << exeDir << "\n";
            ts << "cwd: " << QDir::currentPath() << "\n";
            logFile.close();
        }
    }

    if (!loadDLL()) {
        qCritical() << "Cannot load oneproxy.dll. Current dir:" << QDir::currentPath();
        return 1;
    }
    qDebug() << "DLL loaded OK";

    if (!QFile::exists("config.json")) {
        qCritical() << "config.json not found in" << QDir::currentPath();
        return 1;
    }
    qDebug() << "config.json found";

    OneProxyTray tray;
    return app.exec();
}

#include "main.moc"
