// OneProxy Tray — C++17 Qt6 + Go DLL
// Build: cmake + nmake with MSVC 2022
#include <QApplication>
#include <QSystemTrayIcon>
#include <QMenu>
#include <QTimer>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QFile>
#include <QDebug>
#include <thread>
#include <windows.h>
#include "i18n.h"

// ─── DLL bindings ──────────────────────────────────
typedef char* (*PFN_Start)(char*);
typedef char* (*PFN_Stop)();
typedef char* (*PFN_Restart)();
typedef char* (*PFN_Status)();
typedef char* (*PFN_Check)();
typedef char* (*PFN_Flush)();
typedef char* (*PFN_Select)(char*);
typedef void  (*PFN_Free)(char*);

static PFN_Start  pStart;
static PFN_Stop   pStop;
static PFN_Restart pRestart;
static PFN_Status pStatus;
static PFN_Check  pCheck;
static PFN_Flush  pFlush;
static PFN_Select pSelect;
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
    L(pSelect,"OneProxy_SelectProxy"); L(pFree,"OneProxy_FreeString");
    #undef L
    return true;
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

static QString exeDir() {
    wchar_t b[MAX_PATH]; GetModuleFileNameW(nullptr, b, MAX_PATH);
    QString p = QString::fromWCharArray(b);
    return p.left(p.lastIndexOf('\\'));
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
    OneProxyTray(QSystemTrayIcon *t, QTimer *tm, QObject *parent = nullptr) : QObject(parent), tray(t), timer(tm) {
        tray->setIcon(icoRed);
        tray->setToolTip("OneProxy");
        tray->show();
        connect(timer, &QTimer::timeout, this, &OneProxyTray::tick);
        timer->start(5000);
        QTimer::singleShot(600, this, &OneProxyTray::autoStart);

        // Periodically re-show tray (fixes disappearing icon on taskbar restart)
        QTimer *keepAlive = new QTimer(this);
        connect(keepAlive, &QTimer::timeout, this, [this]() { tray->show(); });
        keepAlive->start(3000);
    }

private:
    void autoStart() {
        auto err = callFree(pStart((char*)"config.json"));
        if (!err.isEmpty()) { qDebug() << "Start failed:" << err; tick(); }
        else {
            qDebug() << "OneProxy: started";
            QTimer::singleShot(3000, this, [this]() {
                std::thread([this]() { callFree(pCheck()); }).detach();
                QTimer::singleShot(8000, this, &OneProxyTray::tick);
            });
        }
    }

    void tick() {
        auto json = callFree(pStatus());
        if (json.isEmpty()) return;
        auto obj = QJsonDocument::fromJson(json.toUtf8()).object();
        bool running = obj["running"].toBool();
        int unifiedPort = obj["unified_port"].toInt();
        auto proxies = obj["proxies"].toArray();

        // Find lowest-latency healthy proxy
        int ok = 0, total = 0, minLat = 999999;
        QString active;
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

        // Icon
        if (!running)      { tray->setIcon(icoRed); tray->setToolTip("OneProxy — stopped"); }
        else if (ok == total) { tray->setIcon(icoGreen); tray->setToolTip(QString("OneProxy %1/%2 OK").arg(ok).arg(total)); }
        else if (ok > 0)  { tray->setIcon(icoYellow); tray->setToolTip(QString("OneProxy %1/%2").arg(ok).arg(total)); }
        else                { tray->setIcon(icoRed); tray->setToolTip("OneProxy — all down"); }

        auto s = getStrings();
        auto* menu = new QMenu();

        // Unified header row
        if (unifiedPort > 0 && running) {
            QString hdr = active.isEmpty() ?
                s.unifiedLabel.arg(unifiedPort) :
                s.unifiedLabel.arg(QString(":%1 ▶ %2 (%3ms)").arg(unifiedPort).arg(active).arg(minLat));
            menu->addAction(hdr)->setEnabled(false);
            menu->addSeparator();
        }

        // Proxy list
        for (const auto& p : proxies) {
            auto px = p.toObject();
            if (!px["enabled"].toBool()) continue;
            QString name = px["name"].toString();
            bool h = px["is_healthy"].toBool();
            int port = px["port"].toInt();
            int lat = px["latency_ms"].toInt();
            bool isActive = (unifiedPort > 0 && h && name == active);

            QString label = isActive ? QString("  ▶ %1  :%2  %3ms").arg(name, -16).arg(port, 5).arg(lat)
                         : h        ? QString("    %1  :%2  %3ms").arg(name, -16).arg(port, 5).arg(lat)
                         :            QString("  ✗ %1  :%2  %3").arg(name, -16).arg(port, 5).arg(s.timeout);

            auto* a = menu->addAction(label);
            if (h && unifiedPort > 0 && !isActive) {
                // Clickable — switch unified to this node
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
        menu->addAction(s.quit, this, &OneProxyTray::doQuit);

        delete tray->contextMenu();
        tray->setContextMenu(menu);
    }

    void selectProxy(const QString &name) {
        std::thread([this, name]() {
            QByteArray ba = name.toUtf8();
            callFree(pSelect(ba.data()));
            QTimer::singleShot(500, this, &OneProxyTray::tick);
        }).detach();
    }

    void doStart()    { callFree(pStart((char*)"config.json")); tick(); }
    void doStop()     { callFree(pStop()); tick(); }
    void doRestart()  {
        callFree(pRestart());
        QTimer::singleShot(3000, this, [this]() {
            std::thread([this]() { callFree(pCheck()); }).detach();
            QTimer::singleShot(8000, this, &OneProxyTray::tick);
        });
    }
    void doCheck()    { std::thread([this]() { callFree(pCheck()); QTimer::singleShot(0, this, &OneProxyTray::tick); }).detach(); }
    void doFlush()    { callFree(pFlush()); QTimer::singleShot(2000, this, &OneProxyTray::tick); }
    void doQuit()     { callFree(pStop()); tray->hide(); QApplication::quit(); }

private:
    QSystemTrayIcon *tray;
    QTimer *timer;
};

int main(int argc, char *argv[]) {
    QApplication app(argc, argv);
    app.setQuitOnLastWindowClosed(false);
    qDebug() << "loading icons...";
    icoGreen  = loadIcon("green.ico");  qDebug() << "  green ok";
    icoYellow = loadIcon("yellow.ico"); qDebug() << "  yellow ok";
    icoRed    = loadIcon("red.ico");    qDebug() << "  red ok";
    if (!loadDLL()) { qCritical() << "DLL failed"; return 1; }
    qDebug() << "DLL OK";
    if (!QFile::exists("config.json")) { qCritical() << "no config.json"; return 1; }

    auto *tray = new QSystemTrayIcon();
    auto *timer = new QTimer();
    new OneProxyTray(tray, timer, &app);
    return app.exec();
}

#include "main.moc"
