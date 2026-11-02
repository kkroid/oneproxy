#include "upgrade.h"
#include "instance_guard.h"

#include <QCoreApplication>
#include <QDebug>
#include <QDir>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QLocalSocket>
#include <QProcess>
#include <QTemporaryDir>
#include <QThread>
#include <QTimer>
#ifdef Q_OS_MACOS
#include <unistd.h>
#endif

static QString socketPath(const QString &directory)
{
    return QDir(directory).filePath("upgrade.socket");
}

bool listenForUpgrade(QLocalServer &server, const QString &directory,
                      const std::function<QString()> &stop)
{
    // Only the instance-lock owner may remove a stale endpoint.
    QLocalServer::removeServer(socketPath(directory));
    server.setSocketOptions(QLocalServer::UserAccessOption);
    QObject::connect(&server, &QLocalServer::newConnection, &server, [&server, stop]() {
        while (auto *socket = server.nextPendingConnection()) {
            socket->setReadBufferSize(64);
            QTimer::singleShot(2000, socket, &QObject::deleteLater);
            QObject::connect(socket, &QLocalSocket::readyRead, socket, [socket, stop]() {
                if (!socket->canReadLine()) return;
                if (socket->readLine() != "quit-for-upgrade\n") {
                    socket->disconnectFromServer();
                    return;
                }
                // No acknowledgement until the real core and startup worker stop.
                const QString error = stop();
                socket->write(error.isEmpty() ? "stopped\n" : "stop-failed\n");
                socket->flush();
                socket->disconnectFromServer();
                if (error.isEmpty()) QCoreApplication::quit();
                else qWarning() << "Upgrade refused: core cleanup failed:" << error;
            });
        }
    });
    return server.listen(socketPath(directory));
}

int installMacUpdate(const QString &source, const QString &destination,
                     const QString &directory)
{
#ifdef Q_OS_MACOS
    if (geteuid() == 0) {
        qCritical() << "Run the installer as the current desktop user, not root.";
        return 1;
    }
#endif
    const QFileInfo from(source), to(destination);
    const QString src = from.canonicalFilePath();
    const QString parent = QFileInfo(to.absolutePath()).canonicalFilePath();
    const QString dst = QDir(parent).filePath(to.fileName());
    if (src.isEmpty() || parent.isEmpty() || from.isSymLink() || to.isSymLink() ||
        to.fileName() != "oneproxy-tray.app" || from.fileName() != "oneproxy-tray.app" ||
        (to.exists() && !to.isDir()) ||
        src == dst || src.startsWith(dst + "/") || dst.startsWith(src + "/") ||
        !QFileInfo(src + "/Contents/MacOS/oneproxy-tray").isExecutable()) {
        qCritical() << "Invalid source/destination app. Use a separate extracted bundle and an existing destination parent.";
        return 1;
    }

    // Stage before disrupting the running proxy. Never merge files into an old app.
    QTemporaryDir staging(QDir(parent).filePath(".oneproxy-update-XXXXXX"));
    if (!staging.isValid()) {
        qCritical() << "Cannot write destination directory:" << parent;
        return 1;
    }
    const QString staged = staging.path() + "/oneproxy-tray.app";
    if (QProcess::execute("/usr/bin/ditto", {src, staged}) != 0 ||
        QProcess::execute("/usr/bin/codesign", {"--verify", "--deep", "--strict", staged}) != 0) {
        qCritical() << "Copy/signature verification failed; installed app was not changed.";
        return 1;
    }

    InstanceGuard guard(directory);
    QString lockError = guard.acquire();
    if (!lockError.isEmpty()) {
        QLocalSocket socket;
        socket.connectToServer(socketPath(directory));
        if (!socket.waitForConnected(1000)) {
            qCritical() << "Quit the old OneProxy from its menu, then retry. This version may not support upgrade handoff.";
            return 1;
        }
        socket.write("quit-for-upgrade\n");
        socket.flush();
        QElapsedTimer deadline;
        deadline.start();
        QByteArray reply;
        while (!reply.contains('\n') && deadline.elapsed() < 15000) {
            if (socket.bytesAvailable() == 0 && !socket.waitForReadyRead(1000)) {
                if (socket.state() == QLocalSocket::UnconnectedState) break;
            }
            reply += socket.readAll();
        }
        if (reply != "stopped\n") {
            qCritical() << "Old instance did not confirm cleanup; app was not replaced.";
            return 1;
        }
        // An IPC reply alone is insufficient: wait for the lifetime lock too.
        while (!(lockError = guard.acquire()).isEmpty() && deadline.elapsed() < 15000)
            QThread::msleep(50);
        if (!lockError.isEmpty()) {
            qCritical() << "Old instance has not exited; app was not replaced.";
            return 1;
        }
    }
    // Keep the same per-user lock across replacement; another copy cannot start.
    const QString backup = staging.path() + "/previous.app";
    const bool existed = QFileInfo::exists(dst);
    if (existed && !QDir().rename(dst, backup)) {
        qCritical() << "Cannot move installed app; no replacement performed.";
        return 1;
    }
    if (!QDir().rename(staged, dst)) {
        if (existed && !QDir().rename(backup, dst)) {
            staging.setAutoRemove(false);
            qCritical() << "Restore failed. Previous app preserved at:" << backup;
        }
        qCritical() << "Cannot install new app.";
        return 1;
    }
    qInfo() << "Installed:" << dst << "Open it to reconnect.";
    return 0;
}
