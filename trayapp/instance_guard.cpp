#include "instance_guard.h"

#include <QCoreApplication>
#include <QDir>
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

InstanceGuard::InstanceGuard(const QString &stateDirectory)
    : directory(stateDirectory), lock(QDir(directory).filePath("oneproxy-tray.lock"))
{
    // A long-running tray must never lose its lock just because it is old.
    lock.setStaleLockTime(0);
}

QString InstanceGuard::acquire()
{
    if (!QDir().mkpath(directory)) {
        return QStringLiteral("Cannot create OneProxy state directory: %1").arg(directory);
    }
    if (!lock.tryLock(0)) {
        if (lock.error() == QLockFile::LockFailedError) {
            qint64 pid = 0;
            QString host, name;
            lock.getLockInfo(&pid, &host, &name);
            return QStringLiteral("OneProxy is already running (PID %1). Use its tray icon, "
                                  "or quit it before launching this copy.").arg(pid);
        }
        return QStringLiteral("Cannot lock OneProxy state directory: %1").arg(directory);
    }
    return {};
}

QList<qint64> legacyTrayProcesses()
{
    QList<qint64> processes;
#ifdef Q_OS_WIN
    HWND window = nullptr;
    while ((window = FindWindowExW(HWND_MESSAGE, window, L"OneProxyTrayWindow", nullptr))) {
        DWORD pid = 0;
        GetWindowThreadProcessId(window, &pid);
        if (pid && pid != GetCurrentProcessId() && !processes.contains(pid)) {
            processes.append(pid);
        }
    }
#endif
    return processes;
}
