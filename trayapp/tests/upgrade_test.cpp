#include "../upgrade.h"
#include "../instance_guard.h"
#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QLocalSocket>
#include <QProcess>
#include <QTemporaryDir>
#include <QTest>

class UpgradeTest : public QObject {
    Q_OBJECT
private slots:
    void installWaitsForCleanup() { verifyInstall(false); }
    void failedCleanupPreservesApp() { verifyInstall(true); }
    void legacyOwnerPreservesApp() { verifyInstall(false, true); }

private:
    void verifyInstall(bool fail, bool legacy = false) {
#ifndef Q_OS_MACOS
        QSKIP("Real bundle replacement requires macOS ditto/codesign");
#else
        QTemporaryDir temp;
        QVERIFY(temp.isValid());
        const QString source = temp.path() + "/download/oneproxy-tray.app";
        const QString target = temp.path() + "/installed/oneproxy-tray.app";
        const QString state = temp.path() + "/state";
        QVERIFY(QDir().mkpath(source + "/Contents/MacOS"));
        QVERIFY(QDir().mkpath(target));
        QVERIFY(QFile::copy("/bin/sleep", source + "/Contents/MacOS/oneproxy-tray"));
        QFile plist(source + "/Contents/Info.plist");
        QVERIFY(plist.open(QIODevice::WriteOnly));
        plist.write("<?xml version=\"1.0\"?><plist version=\"1.0\"><dict>"
                    "<key>CFBundleExecutable</key><string>oneproxy-tray</string>"
                    "<key>CFBundleIdentifier</key><string>test.oneproxy.upgrade</string>"
                    "<key>CFBundlePackageType</key><string>APPL</string></dict></plist>");
        plist.close();
        QCOMPARE(QProcess::execute("/usr/bin/codesign", {"--force", "--sign", "-", source}), 0);
        QFile sentinel(target + "/old-version");
        QVERIFY(sentinel.open(QIODevice::WriteOnly));
        sentinel.write("old");
        sentinel.close();

        QProcess owner;
        owner.start(QCoreApplication::applicationFilePath(),
                    {"--owner", state, legacy ? "legacy" : fail ? "fail" : "ok"});
        QVERIFY(owner.waitForStarted());
        QVERIFY2(owner.waitForReadyRead(5000), owner.readAllStandardError().constData());
        QCOMPARE(owner.readLine().trimmed(), QByteArray("ready"));
        const int result = installMacUpdate(source, target, state);
        if (fail || legacy) {
            QCOMPARE(result, 1);
            QVERIFY(QFile::exists(target + "/old-version"));
            QCOMPARE(owner.state(), QProcess::Running);
            owner.kill(); // Isolated helper only, never a real proxy.
            QVERIFY(owner.waitForFinished());
        } else {
            QCOMPARE(result, 0);
            QVERIFY(owner.waitForFinished(5000) || owner.state() == QProcess::NotRunning);
            QVERIFY(QFile::exists(state + "/child-reaped"));
            QVERIFY(!QFile::exists(target + "/old-version"));
            QVERIFY(QFile::exists(target + "/Contents/MacOS/oneproxy-tray"));
            InstanceGuard guard(state);
            QVERIFY(guard.acquire().isEmpty());
        }
#endif
    }
};

int main(int argc, char **argv) {
    QCoreApplication app(argc, argv);
    if (app.arguments().size() == 4 && app.arguments()[1] == "--owner") {
        const auto directory = app.arguments()[2];
        const auto mode = app.arguments()[3];
        InstanceGuard guard(directory);
        if (!guard.acquire().isEmpty()) return 2;
        QLocalServer server;
        QProcess child;
        if (mode == "ok") {
            child.start("/bin/sleep", {"60"});
            if (!child.waitForStarted()) return 3;
        }
        if (mode != "legacy" && !listenForUpgrade(server, directory, [&]() -> QString {
            if (mode == "fail") return "simulated stop failure";
            child.terminate();
            if (!child.waitForFinished(3000)) return "child still running";
            QFile marker(directory + "/child-reaped");
            if (!marker.open(QIODevice::WriteOnly)) return "marker failed";
            return {};
        })) {
            qCritical() << server.errorString();
            return 4;
        }
        fprintf(stdout, "ready\n");
        fflush(stdout);
        return app.exec();
    }
    UpgradeTest test;
    return QTest::qExec(&test, argc, argv);
}

#include "upgrade_test.moc"
